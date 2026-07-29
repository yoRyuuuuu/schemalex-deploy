package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/shogo82148/schemalex-deploy/mycnf"
)

// ExecMode execute mode
type ExecMode string

const (
	// ExecModeDeploy deploy mode
	ExecModeDeploy ExecMode = "deploy"
	// ExecModeImport import mode
	ExecModeImport ExecMode = "import"
)

type config struct {
	Version     bool
	Socket      string
	Host        string
	User        string
	Password    string
	Database    string
	Port        int
	TLS         string
	Schema      []byte
	AutoApprove bool
	DryRun      bool
	Mode        ExecMode
}

// for testing
var loadDefault = mycnf.LoadDefault

// tlsFromSSLMode converts the mysql(1) --ssl-mode value into the value
// the go-sql-driver/mysql accepts as Config.TLSConfig.
// https://dev.mysql.com/doc/refman/8.0/en/connection-options.html#option_general_ssl-mode
func tlsFromSSLMode(mode string) (string, error) {
	switch strings.ToUpper(mode) {
	case "DISABLED":
		return "false", nil
	case "PREFERRED":
		return "preferred", nil
	case "REQUIRED":
		// the connection is encrypted, but the server certificate is not verified.
		return "skip-verify", nil
	case "VERIFY_IDENTITY":
		// tls=true verifies both the certificate chain and the host name.
		return "true", nil
	case "VERIFY_CA":
		// VERIFY_CA verifies the certificate chain but not the host name.
		// The go-sql-driver/mysql cannot express it without registering a
		// custom tls.Config, and tls=true would verify the host name too,
		// rejecting a server that mysql(1) accepts.
		return "", errors.New("ssl-mode=VERIFY_CA is not supported; use VERIFY_IDENTITY to verify the host name too, or REQUIRED to skip the verification")
	}
	return "", fmt.Errorf("invalid ssl-mode in the configure file: %q", mode)
}

func loadConfig(args []string) (*config, error) {
	var cfn config
	var version bool
	var socket string
	var host, username, password, database string
	var port int
	var tls string
	var approve bool
	var dryRun bool
	var runImport bool

	flagSet := flag.NewFlagSet(args[0], flag.ExitOnError)

	flagSet.Usage = func() {
		fmt.Printf(`schemalex-deploy version %s

-socket           the unix domain socket path for the database
-host             the host name of the database
-port             the port number(default: 3306)
-user             username
-password         password
-database         the database name
-tls              TLS mode: true, false, skip-verify, preferred
-version          show the version
-auto-approve     skips interactive approval of plan before deploying
-dry-run          outputs the schema difference, and then exit the program
-import           imports existing table schemas from running database
`, getVersion())
	}

	// options that are compatible with the mysql(1)
	// https://dev.mysql.com/doc/refman/8.0/en/mysql-command-options.html
	flagSet.StringVar(&socket, "socket", "", "the unix domain socket path for the database")
	flagSet.StringVar(&host, "host", "", "the host name of the database")
	flagSet.IntVar(&port, "port", 0, "the port number")
	flagSet.StringVar(&username, "user", "", "username")
	flagSet.StringVar(&password, "password", "", "password")
	flagSet.StringVar(&database, "database", "", "the database name")
	flagSet.StringVar(&tls, "tls", "", "TLS mode for the connection (true, false, skip-verify, preferred)")
	flagSet.BoolVar(&version, "version", false, "show the version")

	// for schemalex-deploy
	flagSet.BoolVar(&approve, "auto-approve", false, "skips interactive approval of plan before deploying")
	flagSet.BoolVar(&dryRun, "dry-run", false, "outputs the schema difference, and then exit the program")
	flagSet.BoolVar(&runImport, "import", false, "imports existing table schemas from running database")
	if err := flagSet.Parse(args[1:]); err != nil {
		return nil, err
	}

	if version {
		cfn.Version = true
		return &cfn, nil
	}

	cfn.AutoApprove = approve
	cfn.DryRun = dryRun
	cfn.Port = 3306

	// choose execute mode
	cfn.Mode = ExecModeDeploy
	if runImport {
		cfn.Mode = ExecModeImport
	}

	// load configure from files
	cnfFile, err := loadDefault("")
	if err != nil {
		return nil, err
	}
	if client, ok := cnfFile["client"]; ok {
		if v, ok := client["socket"]; ok {
			cfn.Socket = v
		}
		if v, ok := client["host"]; ok {
			cfn.Host = v
		}
		if v, ok := client["port"]; ok {
			if i, err := strconv.Atoi(v); err == nil { // if NO error
				cfn.Port = i
			}
		}
		if v, ok := client["user"]; ok {
			cfn.User = v
		}
		if v, ok := client["password"]; ok {
			cfn.Password = v
		}
		if v, ok := client["database"]; ok {
			cfn.Database = v
		}
		if v, ok := client["ssl-mode"]; ok {
			// an unknown ssl-mode is an error rather than ignored;
			// silently ignoring it would fall back to a plaintext connection.
			mode, err := tlsFromSSLMode(v)
			if err != nil {
				return nil, err
			}
			cfn.TLS = mode
		}
	}

	// load configure from the environment values
	// https://dev.mysql.com/doc/refman/8.0/en/environment-variables.html
	if v := os.Getenv("MYSQL_UNIX_PORT"); v != "" {
		cfn.Socket = v
	}
	if v := os.Getenv("MYSQL_HOST"); v != "" {
		cfn.Host = v
	}
	if v := os.Getenv("MYSQL_PWD"); v != "" {
		cfn.Password = v
	}
	if runtime.GOOS == "windows" {
		if v := os.Getenv("USER"); v != "" {
			cfn.User = v
		}
	} else {
		if cfn.User == "" {
			if u, err := user.Current(); err == nil { // if NO error
				cfn.User = u.Username
			}
		}
	}
	if v := os.Getenv("MYSQL_TCP_PORT"); v != "" {
		if i, err := strconv.Atoi(v); err == nil { // if NO error
			cfn.Port = i
		}
	}

	if socket != "" {
		cfn.Socket = socket
	}
	if host != "" {
		cfn.Host = host
	}
	if port != 0 {
		cfn.Port = port
	}
	if username != "" {
		cfn.User = username
	}
	if password != "" {
		cfn.Password = password
	}
	if database != "" {
		cfn.Database = database
	}
	if tls != "" {
		// This tool never calls mysql.RegisterTLSConfig, so any other value
		// would fail at connect time with a message about the TLS config
		// registry, which says nothing about the -tls flag.
		//
		// Keep this list to the canonical spellings. The driver also treats
		// "1", "TRUE" and "True" as full verification, and the unix domain
		// socket check below only recognizes "true"; accepting the aliases
		// here would let them slip past it.
		switch tls {
		case "true", "false", "skip-verify", "preferred":
		default:
			return nil, fmt.Errorf("invalid -tls value: %q (must be one of true, false, skip-verify, preferred)", tls)
		}
		cfn.TLS = tls
	}

	// The go-sql-driver/mysql derives tls.Config.ServerName from the address.
	// A unix domain socket path has no host name to derive it from, so the
	// driver leaves it empty and the handshake fails with an error that
	// mentions neither the socket nor the TLS setting. Reject it here instead.
	// A socket connection is local and protected by file permissions, so
	// encrypting it adds no security.
	// https://dev.mysql.com/doc/refman/8.4/en/using-encrypted-connections.html
	if cfn.Socket != "" && cfn.TLS == "true" {
		return nil, errors.New("TLS certificate verification is not supported with a unix domain socket, and encrypting a socket connection adds no security; remove -tls, or connect with -host to verify the certificate")
	}

	// deploy mode: load schema file
	if cfn.Mode == ExecModeDeploy {
		if flagSet.NArg() == 0 {
			flagSet.Usage()
			return nil, errors.New("schema file is required")
		}
		schema, err := os.ReadFile(flagSet.Arg(0))
		if err != nil {
			return nil, err
		}
		cfn.Schema = schema
	}

	return &cfn, nil
}

// dsn builds the data source name for the go-sql-driver/mysql.
func (cfn *config) dsn() string {
	c := mysql.NewConfig()
	if cfn.Socket != "" {
		c.Net = "unix"
		c.Addr = cfn.Socket
	} else {
		c.Net = "tcp"
		c.Addr = net.JoinHostPort(cfn.Host, strconv.Itoa(cfn.Port))
	}
	c.User = cfn.User
	c.Passwd = cfn.Password
	c.DBName = cfn.Database
	c.TLSConfig = cfn.TLS
	c.ParseTime = true
	c.RejectReadOnly = true
	c.Params = map[string]string{
		"charset": "utf8mb4",
		// kamipo TRADITIONAL http://www.songmu.jp/riji/entry/2015-07-08-kamipo-traditional.html
		"sql_mode": "'TRADITIONAL,NO_AUTO_VALUE_ON_ZERO,ONLY_FULL_GROUP_BY'",
	}
	return c.FormatDSN()
}
