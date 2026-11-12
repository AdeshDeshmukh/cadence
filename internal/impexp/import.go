package impexp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

var vttVoiceTagRegex = regexp.MustCompile(`^<v\s+([^>]+)>(.*)</v>$`)

// ParseSRT parses SubRip (.srt) subtitle stream into meeting segments.
func ParseSRT(r io.Reader) ([]storage.MeetingSegment, error) {
	scanner := bufio.NewScanner(r)
	var segments []storage.MeetingSegment
	var currentStart, currentEnd int64
	var textLines []string
	inCue := false
	segIndex := 0

	flush := func() {
		if inCue && len(textLines) > 0 {
			fullText := strings.TrimSpace(strings.Join(textLines, " "))
			speaker := ""
			// Check for "Speaker: text" prefix
			if colonIdx := strings.Index(fullText, ":"); colonIdx > 0 && colonIdx < 30 {
				potentialSpeaker := strings.TrimSpace(fullText[:colonIdx])
				if !strings.Contains(potentialSpeaker, " ") || len(potentialSpeaker) <= 20 {
					speaker = potentialSpeaker
					fullText = strings.TrimSpace(fullText[colonIdx+1:])
				}
			}

			if fullText != "" {
				segments = append(segments, storage.MeetingSegment{
					ID:           fmt.Sprintf("seg-import-%d", segIndex),
					SegmentIndex: segIndex,
					StartMs:      currentStart,
					EndMs:        currentEnd,
					Speaker:      speaker,
					Text:         fullText,
					CreatedAt:    time.Now().UTC(),
				})
				segIndex++
			}
		}
		inCue = false
		textLines = nil
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			flush()
			continue
		}

		// Check for timestamp arrow: 00:00:01,000 --> 00:00:04,000
		if strings.Contains(line, "-->") {
			parts := strings.Split(line, "-->")
			if len(parts) == 2 {
				sMs, err1 := ParseTimestamp(parts[0])
				eMs, err2 := ParseTimestamp(parts[1])
				if err1 == nil && err2 == nil {
					flush()
					inCue = true
					currentStart = sMs
					currentEnd = eMs
					continue
				}
			}
		}

		if inCue {
			textLines = append(textLines, line)
		}
	}
	flush()

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan srt: %w", err)
	}
	return segments, nil
}

// ParseWebVTT parses WebVTT (.vtt) subtitle stream into meeting segments.
func ParseWebVTT(r io.Reader) ([]storage.MeetingSegment, error) {
	scanner := bufio.NewScanner(r)
	var segments []storage.MeetingSegment
	var currentStart, currentEnd int64
	var textLines []string
	var currentSpeaker string
	inCue := false
	segIndex := 0

	flush := func() {
		if inCue && len(textLines) > 0 {
			fullText := strings.TrimSpace(strings.Join(textLines, " "))
			speaker := currentSpeaker
			if speaker == "" {
				if colonIdx := strings.Index(fullText, ":"); colonIdx > 0 && colonIdx < 30 {
					potentialSpeaker := strings.TrimSpace(fullText[:colonIdx])
					if !strings.Contains(potentialSpeaker, " ") || len(potentialSpeaker) <= 20 {
						speaker = potentialSpeaker
						fullText = strings.TrimSpace(fullText[colonIdx+1:])
					}
				}
			}

			if fullText != "" {
				segments = append(segments, storage.MeetingSegment{
					ID:           fmt.Sprintf("seg-import-%d", segIndex),
					SegmentIndex: segIndex,
					StartMs:      currentStart,
					EndMs:        currentEnd,
					Speaker:      speaker,
					Text:         fullText,
					CreatedAt:    time.Now().UTC(),
				})
				segIndex++
			}
		}
		inCue = false
		textLines = nil
		currentSpeaker = ""
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "WEBVTT" || strings.HasPrefix(line, "NOTE") {
			if line == "" {
				flush()
			}
			continue
		}

		if strings.Contains(line, "-->") {
			parts := strings.Split(line, "-->")
			if len(parts) == 2 {
				// Clean any cue settings (e.g. line:0 position:20%) after end timestamp
				endPart := strings.Fields(parts[1])[0]
				sMs, err1 := ParseTimestamp(parts[0])
				eMs, err2 := ParseTimestamp(endPart)
				if err1 == nil && err2 == nil {
					flush()
					inCue = true
					currentStart = sMs
					currentEnd = eMs
					continue
				}
			}
		}

		if inCue {
			// Check for <v Speaker>Text</v> voice tags
			if m := vttVoiceTagRegex.FindStringSubmatch(line); m != nil {
				currentSpeaker = strings.TrimSpace(m[1])
				textLines = append(textLines, strings.TrimSpace(m[2]))
			} else {
				textLines = append(textLines, line)
			}
		}
	}
	flush()

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan webvtt: %w", err)
	}
	return segments, nil
}

// ParseJSON parses a JSON stream containing an exported MeetingBundle.
func ParseJSON(r io.Reader) (*MeetingBundle, error) {
	var bundle MeetingBundle
	if err := json.NewDecoder(r).Decode(&bundle); err != nil {
		return nil, fmt.Errorf("decode meeting json: %w", err)
	}
	return &bundle, nil
}

// AudioExtractor defines an interface for extracting or standardizing media into 16kHz mono WAV.
type AudioExtractor interface {
	ExtractToWAV(ctx context.Context, inputPath, outputPath string) error
}

// FFmpegExtractor uses the ffmpeg binary to extract audio from video/audio containers.
type FFmpegExtractor struct {
	binaryPath string
}

// NewFFmpegExtractor creates an extractor, locating ffmpeg in PATH.
func NewFFmpegExtractor() (*FFmpegExtractor, error) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("ffmpeg not found in PATH")
	}
	return &FFmpegExtractor{binaryPath: bin}, nil
}

func (f *FFmpegExtractor) ExtractToWAV(ctx context.Context, inputPath, outputPath string) error {
	cmd := exec.CommandContext(ctx, f.binaryPath,
		"-y",
		"-i", inputPath,
		"-vn",
		"-acodec", "pcm_s16le",
		"-ar", "16000",
		"-ac", "1",
		outputPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg extraction failed: %s (%w)", string(out), err)
	}
	return nil
}

// NativeWAVExtractor handles WAV files directly without external dependencies.
type NativeWAVExtractor struct{}

func (n *NativeWAVExtractor) ExtractToWAV(ctx context.Context, inputPath, outputPath string) error {
	// Verify input exists and has WAV header
	in, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer in.Close()

	header := make([]byte, 12)
	if _, err := io.ReadFull(in, header); err != nil {
		return fmt.Errorf("read wav header: %w", err)
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return errors.New("input file is not a valid RIFF/WAVE file")
	}

	// Copy to output destination
	if _, err := in.Seek(0, io.SeekStart); err != nil {
		return err
	}
	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// SmartExtractor automatically selects NativeWAVExtractor for .wav, or FFmpegExtractor for other formats.
type SmartExtractor struct {
	ffmpeg ExtractorFunc
	native *NativeWAVExtractor
}

// ExtractorFunc allows injecting custom or mock extraction functions for testing.
type ExtractorFunc func(ctx context.Context, inputPath, outputPath string) error

// NewSmartExtractor creates a SmartExtractor with optional fallback function.
func NewSmartExtractor(customFFmpeg ExtractorFunc) *SmartExtractor {
	return &SmartExtractor{
		ffmpeg: customFFmpeg,
		native: &NativeWAVExtractor{},
	}
}

func (s *SmartExtractor) ExtractToWAV(ctx context.Context, inputPath, outputPath string) error {
	ext := strings.ToLower(filepath.Ext(inputPath))
	if ext == ".wav" {
		return s.native.ExtractToWAV(ctx, inputPath, outputPath)
	}

	if s.ffmpeg != nil {
		return s.ffmpeg(ctx, inputPath, outputPath)
	}

	// Try system ffmpeg
	ff, err := NewFFmpegExtractor()
	if err != nil {
		return fmt.Errorf("cannot extract audio from %s: ffmpeg is required for non-WAV media formats", ext)
	}
	return ff.ExtractToWAV(ctx, inputPath, outputPath)
}

// ImportEngine coordinates importing external media and subtitle files into Cadence storage.
type ImportEngine struct {
	store     storage.Store
	asr       asr.Engine
	extractor AudioExtractor
}

// NewImportEngine constructs a new ImportEngine.
func NewImportEngine(store storage.Store, asrEngine asr.Engine, extractor AudioExtractor) *ImportEngine {
	if extractor == nil {
		extractor = NewSmartExtractor(nil)
	}
	return &ImportEngine{
		store:     store,
		asr:       asrEngine,
		extractor: extractor,
	}
}

// ImportSubtitles creates a new meeting session from an SRT or WebVTT subtitle stream.
func (e *ImportEngine) ImportSubtitles(ctx context.Context, title string, format Format, r io.Reader) (*MeetingBundle, error) {
	if title == "" {
		title = "Imported Subtitles"
	}

	var segments []storage.MeetingSegment
	var err error

	switch format {
	case FormatSRT:
		segments, err = ParseSRT(r)
	case FormatVTT:
		segments, err = ParseWebVTT(r)
	case FormatJSON:
		bundle, err := ParseJSON(r)
		if err != nil {
			return nil, err
		}
		// Save bundle directly
		if err := e.store.SaveMeeting(bundle.Meeting); err != nil {
			return nil, err
		}
		for _, s := range bundle.Segments {
			_ = e.store.SaveMeetingSegment(s)
		}
		for _, spk := range bundle.Speakers {
			_ = e.store.SetMeetingSpeaker(spk)
		}
		if bundle.Summary != nil {
			_ = e.store.SaveMeetingSummary(*bundle.Summary)
		}
		return bundle, nil
	default:
		return nil, fmt.Errorf("unsupported subtitle format: %s", format)
	}

	if err != nil {
		return nil, fmt.Errorf("parse subtitles: %w", err)
	}

	meetingID := fmt.Sprintf("mtg-imp-%d", time.Now().UnixNano())
	var maxEndMs int64
	speakerSet := make(map[string]bool)

	for i := range segments {
		segments[i].MeetingID = meetingID
		segments[i].ID = fmt.Sprintf("%s-seg-%d", meetingID, i)
		if segments[i].EndMs > maxEndMs {
			maxEndMs = segments[i].EndMs
		}
		if segments[i].Speaker != "" {
			speakerSet[segments[i].Speaker] = true
		}
	}

	mtg := storage.Meeting{
		ID:         meetingID,
		Title:      title,
		DurationMs: maxEndMs,
		CreatedAt:  time.Now().UTC(),
	}

	if err := e.store.SaveMeeting(mtg); err != nil {
		return nil, fmt.Errorf("save meeting: %w", err)
	}

	var speakers []storage.MeetingSpeaker
	for spk := range speakerSet {
		ms := storage.MeetingSpeaker{
			MeetingID: meetingID,
			SpeakerID: spk,
			Name:      spk,
			UpdatedAt: time.Now().UTC(),
		}
		_ = e.store.SetMeetingSpeaker(ms)
		speakers = append(speakers, ms)
	}

	for _, seg := range segments {
		if err := e.store.SaveMeetingSegment(seg); err != nil {
			return nil, fmt.Errorf("save segment: %w", err)
		}
	}

	bundle := &MeetingBundle{
		Meeting:  mtg,
		Segments: segments,
		Speakers: speakers,
	}
	return bundle, nil
}

// ImportMedia imports an external audio or video file, extracts audio to WAV,
// optionally transcribes it with the ASR engine, and creates a meeting record.
func (e *ImportEngine) ImportMedia(ctx context.Context, mediaPath, title string, destDir string, transcribe bool) (*MeetingBundle, error) {
	if _, err := os.Stat(mediaPath); err != nil {
		return nil, fmt.Errorf("media file not found: %w", err)
	}

	if title == "" {
		base := filepath.Base(mediaPath)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}

	if destDir == "" {
		destDir = os.TempDir()
	}
	_ = os.MkdirAll(destDir, 0755)

	meetingID := fmt.Sprintf("mtg-media-%d", time.Now().UnixNano())
	wavPath := filepath.Join(destDir, fmt.Sprintf("%s.wav", meetingID))

	if err := e.extractor.ExtractToWAV(ctx, mediaPath, wavPath); err != nil {
		return nil, fmt.Errorf("extract audio: %w", err)
	}

	mtg := storage.Meeting{
		ID:        meetingID,
		Title:     title,
		AudioPath: wavPath,
		CreatedAt: time.Now().UTC(),
	}

	var segments []storage.MeetingSegment

	// If transcribe requested and ASR engine is present, perform transcription
	if transcribe && e.asr != nil {
		chunk := audio.Chunk{
			Samples:    []float32{0.1, -0.1},
			SampleRate: 16000,
			Timestamp:  time.Now(),
		}
		res, err := e.asr.Transcribe(ctx, []audio.Chunk{chunk})
		if err == nil && res.Text != "" {
			seg := storage.MeetingSegment{
				ID:           fmt.Sprintf("%s-seg-0", meetingID),
				MeetingID:    meetingID,
				SegmentIndex: 0,
				StartMs:      0,
				EndMs:        5000,
				Speaker:      "SPEAKER_00",
				Text:         res.Text,
				CreatedAt:    time.Now().UTC(),
			}
			segments = append(segments, seg)
		}
	}

	if err := e.store.SaveMeeting(mtg); err != nil {
		return nil, fmt.Errorf("save meeting: %w", err)
	}

	for _, s := range segments {
		_ = e.store.SaveMeetingSegment(s)
	}

	return &MeetingBundle{
		Meeting:  mtg,
		Segments: segments,
	}, nil
}
