// Examples compile the public configuration and exact-value APIs.
package turborel_test

import (
	"fmt"

	turborel "github.com/bschaatsbergen/turborel-go"
)

// ExampleDecimal shows scanning without a binary floating-point conversion.
func ExampleDecimal() {
	var amount turborel.Decimal
	if err := amount.Scan("12345678901234567890.12"); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amount)
	// Output: 12345678901234567890.12
}

// ExampleOpenConnector validates configuration without opening a connection.
func ExampleOpenConnector() {
	connector, err := turborel.OpenConnector(turborel.Config{User: "ANN"})
	fmt.Println(connector != nil, err)
	// Output: true <nil>
}
