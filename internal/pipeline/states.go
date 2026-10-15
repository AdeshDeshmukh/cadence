package pipeline

type State string

const (
	Idle         State = "idle"
	Recording    State = "recording"
	Transcribing State = "transcribing"
	Processing   State = "processing"
	Pasting      State = "pasting"
	Complete     State = "complete"
	Failed       State = "failed"
	Cancelled    State = "cancelled"
)

func (s State) Terminal() bool {
	return s == Complete || s == Failed || s == Cancelled
}
