package main

import (
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/schemalex-deploy/mycnf"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		cnf     mycnf.MyCnf
		want    *config
		wantErr bool

		// environment variables
		unixPort string
		tcpPort  string
		host     string
		pwd      string
	}{
		{
			name: "show version",
			args: []string{"schemalex-deploy", "-version"},
			cnf:  mycnf.MyCnf{},
			want: &config{
				Version: true,
			},
		},

		// password
		{
			name: "The password specified in the argument takes precedence",
			args: []string{"schemalex-deploy", "-user", "shogo", "-password", "secret", filepath.Join("testdata", "schema.sql")},
			cnf: mycnf.MyCnf{
				"client": map[string]string{
					"user":     "chooblarin",
					"password": "password",
				},
			},
			pwd: "environment",
			want: &config{
				User:     "shogo",
				Password: "secret",
				Port:     3306,
				Schema:   []byte{},
				Mode:     ExecModeDeploy,
			},
		},
		{
			name: "The password specified in the environment values takes precedence",
			args: []string{"schemalex-deploy", "-user", "shogo", filepath.Join("testdata", "schema.sql")},
			cnf: mycnf.MyCnf{
				"client": map[string]string{
					"user":     "chooblarin",
					"password": "password",
				},
			},
			pwd: "environment",
			want: &config{
				User:     "shogo",
				Password: "environment",
				Port:     3306,
				Schema:   []byte{},
				Mode:     ExecModeDeploy,
			},
		},
		{
			name: "The password specified in the configure file takes precedence",
			args: []string{"schemalex-deploy", "-user", "shogo", filepath.Join("testdata", "schema.sql")},
			cnf: mycnf.MyCnf{
				"client": map[string]string{
					"user":     "chooblarin",
					"password": "password",
				},
			},
			want: &config{
				User:     "shogo",
				Password: "password",
				Port:     3306,
				Schema:   []byte{},
				Mode:     ExecModeDeploy,
			},
		},

		// port number
		{
			name: "The port-number specified in the argument takes precedence",
			args: []string{"schemalex-deploy", "-port", "1234", filepath.Join("testdata", "schema.sql")},
			cnf: mycnf.MyCnf{
				"client": map[string]string{
					"port":     "2345",
					"user":     "chooblarin",
					"password": "password",
				},
			},
			tcpPort: "3456",
			want: &config{
				User:     "chooblarin",
				Password: "password",
				Port:     1234,
				Schema:   []byte{},
				Mode:     ExecModeDeploy,
			},
		},
		{
			name: "The port-number specified in the environment values takes precedence",
			args: []string{"schemalex-deploy", filepath.Join("testdata", "schema.sql")},
			cnf: mycnf.MyCnf{
				"client": map[string]string{
					"port":     "2345",
					"user":     "chooblarin",
					"password": "password",
				},
			},
			tcpPort: "3456",
			want: &config{
				User:     "chooblarin",
				Password: "password",
				Port:     3456,
				Schema:   []byte{},
				Mode:     ExecModeDeploy,
			},
		},
		{
			name: "The port-number specified in the configure file values takes precedence",
			args: []string{"schemalex-deploy", filepath.Join("testdata", "schema.sql")},
			cnf: mycnf.MyCnf{
				"client": map[string]string{
					"port":     "2345",
					"user":     "chooblarin",
					"password": "password",
				},
			},
			want: &config{
				User:     "chooblarin",
				Password: "password",
				Port:     2345,
				Schema:   []byte{},
				Mode:     ExecModeDeploy,
			},
		},

		// tls
		{
			name: "The tls specified in the argument takes precedence",
			args: []string{"schemalex-deploy", "-tls", "skip-verify", filepath.Join("testdata", "schema.sql")},
			cnf: mycnf.MyCnf{
				"client": map[string]string{
					"user":     "chooblarin",
					"ssl-mode": "DISABLED",
				},
			},
			want: &config{
				User:   "chooblarin",
				TLS:    "skip-verify",
				Port:   3306,
				Schema: []byte{},
				Mode:   ExecModeDeploy,
			},
		},
		{
			name: "The ssl-mode specified in the configure file is used",
			args: []string{"schemalex-deploy", filepath.Join("testdata", "schema.sql")},
			cnf: mycnf.MyCnf{
				"client": map[string]string{
					"user":     "chooblarin",
					"ssl-mode": "VERIFY_IDENTITY",
				},
			},
			want: &config{
				User:   "chooblarin",
				TLS:    "true",
				Port:   3306,
				Schema: []byte{},
				Mode:   ExecModeDeploy,
			},
		},
		{
			name: "invalid ssl-mode in the configure file is an error",
			args: []string{"schemalex-deploy", filepath.Join("testdata", "schema.sql")},
			cnf: mycnf.MyCnf{
				"client": map[string]string{
					"user":     "chooblarin",
					"ssl-mode": "ENABLED",
				},
			},
			wantErr: true,
		},
		{
			name:    "invalid -tls value is an error",
			args:    []string{"schemalex-deploy", "-tls", "ture", filepath.Join("testdata", "schema.sql")},
			cnf:     mycnf.MyCnf{},
			wantErr: true,
		},
		{
			name:    "tls=true with a unix domain socket is an error",
			args:    []string{"schemalex-deploy", "-socket", "/tmp/mysql.sock", "-tls", "true", filepath.Join("testdata", "schema.sql")},
			cnf:     mycnf.MyCnf{},
			wantErr: true,
		},
		{
			name: "ssl-mode=VERIFY_CA is an error",
			args: []string{"schemalex-deploy", filepath.Join("testdata", "schema.sql")},
			cnf: mycnf.MyCnf{
				"client": map[string]string{
					"ssl-mode": "VERIFY_CA",
				},
			},
			wantErr: true,
		},
		{
			// the socket may come from the configure file while -tls comes
			// from the command line, so the check runs on the merged values.
			name: "tls=true with a socket from the configure file is an error",
			args: []string{"schemalex-deploy", "-tls", "true", filepath.Join("testdata", "schema.sql")},
			cnf: mycnf.MyCnf{
				"client": map[string]string{
					"socket": "/tmp/mysql.sock",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := loadDefault
			loadDefault = func(extraFile string) (mycnf.MyCnf, error) {
				return tt.cnf, nil
			}
			defer func() { loadDefault = orig }()
			t.Setenv("MYSQL_UNIX_PORT", tt.unixPort)
			t.Setenv("MYSQL_TCP_PORT", tt.tcpPort)
			t.Setenv("MYSQL_HOST", tt.host)
			t.Setenv("MYSQL_PWD", tt.pwd)

			got, err := loadConfig(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("want error, but got nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("unexpected config: (-want/+got):\n%s", diff)
			}
		})
	}
}

func TestTLSFromSSLMode(t *testing.T) {
	tests := []struct {
		mode    string
		want    string
		wantErr bool
	}{
		{mode: "DISABLED", want: "false"},
		{mode: "PREFERRED", want: "preferred"},
		{mode: "REQUIRED", want: "skip-verify"},
		{mode: "VERIFY_IDENTITY", want: "true"},
		{mode: "verify_identity", want: "true"}, // ssl-mode is case insensitive
		{mode: "", wantErr: true},
		{mode: "ENABLED", wantErr: true},

		// VERIFY_CA verifies the certificate chain but not the host name,
		// which the go-sql-driver/mysql cannot express.
		{mode: "VERIFY_CA", wantErr: true},
		{mode: "verify_ca", wantErr: true},
	}

	for _, tt := range tests {
		got, err := tlsFromSSLMode(tt.mode)
		if tt.wantErr {
			if err == nil {
				t.Errorf("tlsFromSSLMode(%q): want error, but got nil", tt.mode)
			}
			continue
		}
		if err != nil {
			t.Errorf("tlsFromSSLMode(%q): %v", tt.mode, err)
			continue
		}
		if got != tt.want {
			t.Errorf("tlsFromSSLMode(%q) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

// TestTLSAliasesAreRejected guards the unix domain socket check in loadConfig.
// The driver treats "1", "TRUE" and "True" as full verification, but the check
// only recognizes "true". Accepting the aliases would let them reach a socket
// connection and fail with a confusing error from the driver.
func TestTLSAliasesAreRejected(t *testing.T) {
	for _, v := range []string{"1", "TRUE", "True", "0", "FALSE", "SKIP-VERIFY", "Preferred"} {
		args := []string{"schemalex-deploy", "-tls", v, filepath.Join("testdata", "schema.sql")}

		orig := loadDefault
		loadDefault = func(extraFile string) (mycnf.MyCnf, error) {
			return mycnf.MyCnf{}, nil
		}
		_, err := loadConfig(args)
		loadDefault = orig

		if err == nil {
			t.Errorf("-tls=%s: want error, but got nil", v)
		}
	}
}

// TestConfigDSNWithoutTLS checks that a config without TLS builds the same DSN
// as before the -tls option was introduced.
func TestConfigDSNWithoutTLS(t *testing.T) {
	cfn := &config{
		Host:     "127.0.0.1",
		Port:     3306,
		User:     "root",
		Password: "secret",
		Database: "gotest",
	}
	want := "root:secret@tcp(127.0.0.1:3306)/gotest?parseTime=true&rejectReadOnly=true&charset=utf8mb4&sql_mode=%27TRADITIONAL%2CNO_AUTO_VALUE_ON_ZERO%2CONLY_FULL_GROUP_BY%27"
	if got := cfn.dsn(); got != want {
		t.Errorf("dsn() =\n%q\nwant\n%q", got, want)
	}
}

func TestConfigDSN(t *testing.T) {
	tests := []struct {
		name     string
		cfn      *config
		wantNet  string
		wantAddr string
		wantTLS  string
	}{
		{
			name: "tcp without tls",
			cfn: &config{
				Host: "127.0.0.1", Port: 3306, User: "root", Database: "gotest",
			},
			wantNet:  "tcp",
			wantAddr: "127.0.0.1:3306",
			wantTLS:  "",
		},
		{
			name: "tcp with tls skip-verify",
			cfn: &config{
				Host: "db.example.com", Port: 3306, User: "root", Database: "gotest", TLS: "skip-verify",
			},
			wantNet:  "tcp",
			wantAddr: "db.example.com:3306",
			wantTLS:  "skip-verify",
		},
		{
			name: "tcp with full verification",
			cfn: &config{
				Host: "db.example.com", Port: 3306, User: "root", Database: "gotest", TLS: "true",
			},
			wantNet:  "tcp",
			wantAddr: "db.example.com:3306",
			wantTLS:  "true",
		},
		{
			name: "unix socket with tls preferred",
			cfn: &config{
				Socket: "/tmp/mysql.sock", User: "root", Database: "gotest", TLS: "preferred",
			},
			wantNet:  "unix",
			wantAddr: "/tmp/mysql.sock",
			wantTLS:  "preferred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// parse the DSN back instead of comparing strings; the order of
			// the DSN parameters depends on the driver's implementation.
			cfg, err := mysql.ParseDSN(tt.cfn.dsn())
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Net != tt.wantNet {
				t.Errorf("Net = %q, want %q", cfg.Net, tt.wantNet)
			}
			if cfg.Addr != tt.wantAddr {
				t.Errorf("Addr = %q, want %q", cfg.Addr, tt.wantAddr)
			}
			if cfg.TLSConfig != tt.wantTLS {
				t.Errorf("TLSConfig = %q, want %q", cfg.TLSConfig, tt.wantTLS)
			}
			if !cfg.ParseTime {
				t.Error("ParseTime = false, want true")
			}
			if !cfg.RejectReadOnly {
				t.Error("RejectReadOnly = false, want true")
			}
		})
	}
}
