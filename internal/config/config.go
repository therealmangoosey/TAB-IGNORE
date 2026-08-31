package config

// Config is the small bootstrap configuration surface. Additional fields should
// be added only when they are consumed by a subsystem.
type Config struct {
	LibraryPath string
	ServerAddr string
}

func Default() Config {
	return Config{ServerAddr: "127.0.0.1:8788"}
}
