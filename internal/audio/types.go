package audio

import "time"

type Chunk struct {
	Samples    []float32
	SampleRate int
	Timestamp  time.Time
}

func (c Chunk) Duration() time.Duration {
	if c.SampleRate <= 0 {
		return 0
	}
	return time.Duration(
		float64(len(c.Samples)) / float64(c.SampleRate) *
			float64(time.Second),
	)
}

type Device struct {
	ID         string
	Name       string
	SampleRate int
	Input      bool
	Default    bool
}

type Capture interface {
	Devices() ([]Device, error)
	Start() (<-chan Chunk, error)
	Stop() error
}
