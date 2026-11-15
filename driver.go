// The database/sql entry points keep configuration separate from connections.

package turborel

import (
	"context"
	"database/sql"
	"database/sql/driver"
)

// Driver implements the registered database/sql driver "turborel".
type Driver struct{}

func init() { sql.Register("turborel", &Driver{}) }

// Open connects using a turborel URL. database/sql normally calls OpenConnector.
func (d *Driver) Open(name string) (driver.Conn, error) {
	c, err := d.OpenConnector(name)
	if err != nil {
		return nil, err
	}
	return c.Connect(context.Background())
}

// OpenConnector validates a URL once, before the pool opens any connections.
func (*Driver) OpenConnector(name string) (driver.Connector, error) {
	c, err := ParseDSN(name)
	if err != nil {
		return nil, err
	}
	return OpenConnector(c)
}

// Connector opens equivalent connections for database/sql's pool.
type Connector struct{ config Config }

// OpenConnector validates config and owns a clone of its TLS configuration.
// Use sql.OpenDB with the returned connector. It makes no network requests.
func OpenConnector(config Config) (*Connector, error) {
	c, err := normalize(config)
	if err != nil {
		return nil, err
	}
	return &Connector{config: c}, nil
}

// Connect dials, secures, and authenticates a connection within ctx's lifetime.
func (c *Connector) Connect(ctx context.Context) (driver.Conn, error) {
	if c == nil || c.config.User == "" {
		return nil, invalid("use OpenConnector to initialize a connector")
	}
	login, cancel := context.WithTimeout(ctx, c.config.ConnectTimeout)
	defer cancel()
	stream, err := dial(login, c.config)
	if err != nil {
		return nil, err
	}
	connection := newConn(stream, c.config)
	op, err := connection.start(login)
	if err == nil {
		err = connection.login()
	}
	if op != nil {
		err = op.finish(err)
	}
	if err != nil {
		connection.discard()
		return nil, err
	}
	return connection, nil
}

// Driver returns the driver associated with this connector.
func (*Connector) Driver() driver.Driver { return &Driver{} }

var (
	_ driver.DriverContext      = (*Driver)(nil)
	_ driver.Connector          = (*Connector)(nil)
	_ driver.Conn               = (*conn)(nil)
	_ driver.ConnBeginTx        = (*conn)(nil)
	_ driver.ConnPrepareContext = (*conn)(nil)
	_ driver.ExecerContext      = (*conn)(nil)
	_ driver.QueryerContext     = (*conn)(nil)
	_ driver.NamedValueChecker  = (*conn)(nil)
	_ driver.Pinger             = (*conn)(nil)
	_ driver.SessionResetter    = (*conn)(nil)
	_ driver.Validator          = (*conn)(nil)
)
