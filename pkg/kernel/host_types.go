package kernel

// HostService is one service provided by a component.
type HostService struct {
	Name     string
	Version  string
	Priority int32
	Keys     []string
}

// HostRequirement is one service required by a component.
type HostRequirement struct {
	Name      string
	Version   string
	Selection uint8
}

// HostCommand is one CLI command declared by a component.
type HostCommand struct {
	Name    string
	Summary string
	Usage   string
}

// HostDescriptor is a component plugin descriptor.
type HostDescriptor struct {
	ID       string
	Version  string
	Provides []HostService
	Requires []HostRequirement
	Commands []HostCommand
}
