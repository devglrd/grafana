package server

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grafana/grafana/pkg/infra/fs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/ini.v1"
)

func createGrafDir(t *testing.T) (string, string) {
	t.Helper()

	tmpDir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	rootDir := filepath.Join("..", "..")

	cfgDir := filepath.Join(tmpDir, "conf")
	err = os.MkdirAll(cfgDir, 0755)
	require.NoError(t, err)
	dataDir := filepath.Join(tmpDir, "data")
	logsDir := filepath.Join(tmpDir, "logs")
	pluginsDir := filepath.Join(tmpDir, "plugins")
	publicDir := filepath.Join(tmpDir, "public")
	err = os.MkdirAll(publicDir, 0755)
	require.NoError(t, err)
	emailsDir := filepath.Join(publicDir, "emails")
	err = fs.CopyRecursive(filepath.Join(rootDir, "public", "emails"), emailsDir)
	require.NoError(t, err)
	provDir := filepath.Join(cfgDir, "provisioning")
	provDSDir := filepath.Join(provDir, "datasources")
	err = os.MkdirAll(provDSDir, 0755)
	require.NoError(t, err)
	provNotifiersDir := filepath.Join(provDir, "notifiers")
	err = os.MkdirAll(provNotifiersDir, 0755)
	require.NoError(t, err)
	provPluginsDir := filepath.Join(provDir, "plugins")
	err = os.MkdirAll(provPluginsDir, 0755)
	require.NoError(t, err)
	provDashboardsDir := filepath.Join(provDir, "dashboards")
	err = os.MkdirAll(provDashboardsDir, 0755)
	require.NoError(t, err)

	cfg := ini.Empty()
	dfltSect := cfg.Section("")
	_, err = dfltSect.NewKey("app_mode", "development")
	require.NoError(t, err)

	pathsSect, err := cfg.NewSection("paths")
	require.NoError(t, err)
	_, err = pathsSect.NewKey("data", dataDir)
	require.NoError(t, err)
	_, err = pathsSect.NewKey("logs", logsDir)
	require.NoError(t, err)
	_, err = pathsSect.NewKey("plugins", pluginsDir)
	require.NoError(t, err)

	logSect, err := cfg.NewSection("log")
	require.NoError(t, err)
	_, err = logSect.NewKey("level", "debug")
	require.NoError(t, err)

	serverSect, err := cfg.NewSection("server")
	require.NoError(t, err)
	_, err = serverSect.NewKey("port", "0")
	require.NoError(t, err)

	anonSect, err := cfg.NewSection("auth.anonymous")
	require.NoError(t, err)
	_, err = anonSect.NewKey("enabled", "true")
	require.NoError(t, err)

	cfgPath := filepath.Join(cfgDir, "test.ini")
	err = cfg.SaveTo(cfgPath)
	require.NoError(t, err)

	err = fs.CopyFile(filepath.Join(rootDir, "conf", "defaults.ini"), filepath.Join(cfgDir, "defaults.ini"))
	require.NoError(t, err)

	return tmpDir, cfgPath
}

func startGrafana(t *testing.T) string {
	t.Helper()

	tmpDir, cfgPath := createGrafDir(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server, err := New(Config{
		ConfigFile: cfgPath,
		HomePath:   tmpDir,
		Listener:   listener,
	})
	require.NoError(t, err)

	go func() {
		if err := server.Run(); err != nil {
			t.Log("Server exited uncleanly", "error", err)
		}
	}()
	t.Cleanup(func() {
		server.Shutdown("")
	})

	// Wait for Grafana to be ready
	addr := listener.Addr().String()
	resp, err := http.Get(fmt.Sprintf("http://%s/healthz", addr))
	require.NoError(t, err)
	require.NotNil(t, resp)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	t.Logf("Grafana is listening on %s", addr)

	return addr
}

func TestQueryMetrics(t *testing.T) {
	addr := startGrafana(t)
	resp, err := http.Post(fmt.Sprintf("http://%s/api/ds/query", addr), "application/json", nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	t.Cleanup(func() { resp.Body.Close() })

	builder := strings.Builder{}
	_, err = io.Copy(&builder, resp.Body)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "", builder.String())
}
