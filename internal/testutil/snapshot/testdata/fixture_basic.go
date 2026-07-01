// Package fixture_basic exercises CaptureAPI's basic AST coverage:
// exported function, method with receiver, struct type, block const,
// block var.
package fixture_basic

const MaxRetries = 3

const (
	StatusOK    = 200
	StatusError = 500
)

var DefaultName = "atlas"

var (
	Version = "1.0"
	Debug   = true
)

type Config struct {
	Host string
	Port int
}

func New() *Config             { return nil }
func (c *Config) Host() string { return c.Host }
func (c Config) Port() int     { return c.Port }
