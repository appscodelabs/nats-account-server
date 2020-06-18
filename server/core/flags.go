package core

// Flags defines the various flags you can call the account server with. These are used in main
// and passed down to the server code to process.
type Flags struct {
	ConfigFile string

	Directory string

	ReadOnly bool

	NATSURL string
	Creds   string

	Debug           bool
	Verbose         bool
	DebugAndVerbose bool

	HostPort string

	Primary string // Only used to copy jwt from old account server
}
