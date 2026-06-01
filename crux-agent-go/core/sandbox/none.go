package sandbox

// Sandbox controls what an agent can access.
type Sandbox interface {
	CheckRead(path string) error
	CheckWrite(path string) error
	CheckExec(cmd string) error
	CheckNetwork(url string) error
	Env() []string
}

// None is the no-restriction sandbox for MVU.
type None struct{}

func NewNone() *None { return &None{} }

func (n *None) CheckRead(path string) error    { return nil }
func (n *None) CheckWrite(path string) error   { return nil }
func (n *None) CheckExec(cmd string) error     { return nil }
func (n *None) CheckNetwork(url string) error  { return nil }
func (n *None) Env() []string                  { return nil }
