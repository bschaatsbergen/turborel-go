// Connection configuration is explicit about transport privacy and credentials.

package turborel

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"
)

const (
	defaultAddress        = "127.0.0.1:7422"
	defaultConnectTimeout = 30 * time.Second
	cleanupTimeout        = 5 * time.Second
	cancelTimeout         = time.Second
	authSCRAM             = "scram"
	authPassword          = "password"
	authTrust             = "trust"
	authCertificate       = "certificate"
)

// Config describes one database server. A listener serves one database.
// TCP uses TLS unless Plaintext is explicitly set. TLS is cloned by OpenConnector.
type Config struct {
	// Network is tcp (the default) or unix.
	Network string
	// Address is a host:port or Unix socket path; the default is 127.0.0.1:7422.
	Address string
	// User is the SQL authorization identifier. It is required and case-sensitive.
	User string
	// Password is used by scram and password authentication.
	Password string
	// Auth is scram (the default), password, trust or certificate.
	Auth string
	// TLS configures verified TLS. TCP defaults to TLS with system roots.
	TLS *tls.Config
	// Plaintext explicitly disables TCP TLS; it cannot be combined with TLS.
	Plaintext bool
	// AllowInsecureSCRAM permits SCRAM over plaintext TCP without channel binding.
	AllowInsecureSCRAM bool
	// ConnectTimeout bounds dialing, TLS and login; zero means 30 seconds.
	ConnectTimeout time.Duration
	// ReadTimeout bounds the whole operation, including streamed rows; zero disables it.
	ReadTimeout time.Duration
	// MaxBody bounds each frame in bytes; zero means 16 MiB, the protocol maximum.
	MaxBody uint32
}

// String describes a target without exposing its password or TLS key material.
func (c Config) String() string {
	return fmt.Sprintf("turborel(%s %s, user %q)", c.Network, c.Address, c.User)
}

// GoString also redacts credentials when a config is formatted with %#v.
func (c Config) GoString() string { return c.String() }

func normalize(c Config) (Config, error) {
	if c.Network == "" {
		c.Network = "tcp"
	}
	if c.Network == "unix" && c.Address == "" {
		return c, invalid("a Unix socket path is required")
	}
	if c.Address == "" {
		c.Address = defaultAddress
	}
	if c.Auth == "" {
		c.Auth = authSCRAM
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = defaultConnectTimeout
	}
	if c.MaxBody == 0 {
		c.MaxBody = defaultMaxBody
	}
	if c.User == "" {
		return c, invalid("a user is required")
	}
	if c.Network != "tcp" && c.Network != "unix" {
		return c, unsupported("network")
	}
	if c.ConnectTimeout < 0 || c.ReadTimeout < 0 {
		return c, invalid("timeouts must not be negative")
	}
	if c.MaxBody > defaultMaxBody {
		return c, invalid("maximum frame size exceeds 16 MiB")
	}
	if c.Plaintext && c.TLS != nil {
		return c, invalid("TLS and Plaintext are mutually exclusive")
	}
	if c.Network == "tcp" && !c.Plaintext && c.TLS == nil {
		c.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	if c.TLS != nil {
		c.TLS = c.TLS.Clone()
		if c.TLS.InsecureSkipVerify {
			return c, invalid("TLS certificate verification cannot be disabled")
		}
		if c.TLS.MinVersion < tls.VersionTLS12 {
			c.TLS.MinVersion = tls.VersionTLS12
		}
	}
	return c, validateAuth(c)
}

func validateAuth(c Config) error {
	private := c.Network == "unix" || c.TLS != nil
	switch c.Auth {
	case authTrust:
	case authPassword:
		if !private {
			return invalid("password authentication requires TLS or a Unix socket")
		}
	case authSCRAM:
		if !private && !c.AllowInsecureSCRAM {
			return invalid("SCRAM requires TLS or explicit AllowInsecureSCRAM")
		}
	case authCertificate:
		if c.TLS == nil {
			return invalid("certificate authentication requires TLS")
		}
	default:
		return unsupported("authentication mechanism")
	}
	return nil
}

// ParseDSN reads a connection URL of the form turborel://USER:PASSWORD@HOST:PORT.
// Use net/url.UserPassword to escape reserved characters in credentials. A
// listener serves one database, so the URL has no database path.
//
// Supported query options are:
//   - auth: scram (default), password, certificate or trust.
//   - tls: require (default for TCP) or disable. Certificate verification cannot
//     be disabled independently of TLS.
//   - socket: a Unix socket path, which selects the unix network.
//   - connect_timeout: a Go duration such as 5s; zero selects the 30s default.
//   - read_timeout: a duration for the whole operation; zero disables the limit.
//   - allow_insecure_scram: a boolean permitting SCRAM over plaintext TCP.
//
// Unknown or duplicate options and invalid values return an error. Errors never
// include the original URL because it may contain credentials.
func ParseDSN(dsn string) (Config, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return Config{}, invalid("invalid connection URL")
	}
	if u.Scheme != "turborel" || u.Opaque != "" || u.Fragment != "" {
		return Config{}, invalid("expected a turborel URL")
	}
	if u.Path != "" && u.Path != "/" {
		return Config{}, invalid("a listener serves one database; URL paths are unsupported")
	}
	c := Config{Address: defaultAddress}
	if u.User != nil {
		c.User = u.User.Username()
		c.Password, _ = u.User.Password()
	}
	if u.Host != "" {
		port := u.Port()
		if port == "" {
			port = "7422"
		}
		c.Address = net.JoinHostPort(u.Hostname(), port)
	}
	options, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return c, invalid("invalid connection options")
	}
	for key, values := range options {
		if len(values) != 1 {
			return c, invalid("duplicate connection option")
		}
		if err := setOption(&c, key, values[0]); err != nil {
			return c, err
		}
	}
	return normalize(c)
}

func setOption(c *Config, key, value string) error {
	var err error
	switch key {
	case "auth":
		c.Auth = value
	case "socket":
		c.Network, c.Address = "unix", value
	case "tls":
		switch value {
		case "require":
			c.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
		case "disable":
			c.Plaintext = true
		default:
			return invalid("tls must be require or disable")
		}
	case "connect_timeout":
		c.ConnectTimeout, err = time.ParseDuration(value)
	case "read_timeout":
		c.ReadTimeout, err = time.ParseDuration(value)
	case "allow_insecure_scram":
		c.AllowInsecureSCRAM, err = strconv.ParseBool(value)
	default:
		return invalid("unknown connection option")
	}
	if err != nil {
		return invalid("invalid connection option value")
	}
	return nil
}
