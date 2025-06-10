package driver

import (
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

func Connect() (driver.Conn, error) {
	var (
		// ctx       = context.Background()
		conn, err = clickhouse.Open(&clickhouse.Options{
			Addr: []string{"clickhouse-server:9000"},
			//Addr: []string{"localhost:19000"},
			Auth: clickhouse.Auth{
				Database: "default",
				Username: "default",
				Password: "",
			},
			// ClientInfo: clickhouse.ClientInfo{
			// 	Products: []struct {
			// 		Name    string
			// 		Version string
			// 	}{
			// 		{Name: "an-example-go-client", Version: "2.13.0"},
			// 	},
			// },
			//Debug: true,
			Debugf: func(format string, v ...any) {
				fmt.Printf("CLICKHOUSE: "+format+"\n", v...)
			},
		})
	)

	fmt.Println(err)

	//conn.AddQueryHook(chotel.NewQueryHook())

	if err != nil {
		return nil, err
	}

	// if err := conn.Ping(ctx); err != nil {
	// 	if exception, ok := err.(*clickhouse.Exception); ok {
	// 		fmt.Printf("Exception [%d] %s \n%s\n", exception.Code, exception.Message, exception.StackTrace)
	// 	}
	// 	return nil, err
	//
	return conn, nil
}
