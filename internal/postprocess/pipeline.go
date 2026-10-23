package postprocess

type Pipeline struct {
	steps []Step
}

func New(steps ...Step) *Pipeline {
	return &Pipeline{steps: steps}
}

func (p *Pipeline) Apply(text string, ctx Context) string {
	for _, step := range p.steps {
		text = step.Apply(text, ctx)
	}
	return text
}
