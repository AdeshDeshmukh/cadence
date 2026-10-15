package postprocess

type Context struct {
	Language    string
	Application string
	Mode        string
	Vocabulary  map[string]string
}

type Step interface {
	Name() string
	Apply(string, Context) string
}
