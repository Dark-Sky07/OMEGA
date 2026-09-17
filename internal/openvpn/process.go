package openvpn

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// openvpnDir is the shared root for all openvpn inbound data directories.
func openvpnDir() string {
	return config.GetBinFolderPath() + "/openvpn"
}

// GetBinaryPath locates the openvpn binary: a binary dropped next to the
// panel's own binaries (same convention as xray/mtg) wins, then the system
// PATH. Returns "" when neither exists — the panel degrades gracefully in
// that case (certs/profiles still work; daemons simply do not run).
func GetBinaryPath() string {
	name := "openvpn"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	local := config.GetBinFolderPath() + "/" + name
	if st, err := os.Stat(local); err == nil && !st.IsDir() {
		return local
	}
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return ""
}

// Available reports whether the openvpn binary can be found at all.
func Available() bool {
	return GetBinaryPath() != ""
}

var (
	gracefulStopTimeout = 5 * time.Second
	forceStopTimeout    = 2 * time.Second
)

// procLogWriter consumes the openvpn daemon's stdout/stderr, forwards each
// line to the x-ui log and remembers the most recent line for diagnostics.
type procLogWriter struct {
	mu       sync.Mutex
	label    string
	buf      string
	lastLine string
}

func (w *procLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf += string(p)
	for {
		i := strings.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := w.buf[:i]
		w.buf = w.buf[i+1:]
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed != "" {
			w.lastLine = trimmed
			logger.Infof("openvpn: %s | %s", w.label, trimmed)
		}
	}
	return len(p), nil
}

func (w *procLogWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buf != "" {
		trimmed := strings.TrimSpace(strings.TrimRight(w.buf, "\r"))
		if trimmed != "" {
			w.lastLine = trimmed
			logger.Infof("openvpn: %s | %s", w.label, trimmed)
		}
		w.buf = ""
	}
}

func (w *procLogWriter) LastLine() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastLine
}

// Process wraps a single openvpn daemon for one inbound.
type Process struct {
	cmd             *exec.Cmd
	done            chan struct{}
	configPath      string
	logWriter       *procLogWriter
	exitErr         error
	intentionalStop atomic.Bool
}

func newProcess(configPath, label string) *Process {
	return &Process{
		configPath: configPath,
		logWriter:  &procLogWriter{label: label},
	}
}

// IsRunning reports whether the daemon process is currently running.
func (p *Process) IsRunning() bool {
	if p.cmd == nil || p.cmd.Process == nil {
		return false
	}
	if p.done != nil {
		select {
		case <-p.done:
			return false
		default:
		}
	}
	return p.cmd.ProcessState == nil
}

// GetResult returns the last log line or the exit error from the daemon.
func (p *Process) GetResult() string {
	if line := p.logWriter.LastLine(); line != "" {
		return line
	}
	if p.exitErr != nil {
		return p.exitErr.Error()
	}
	return ""
}

// Start launches the daemon against its generated config file. Before
// starting, any daemon a previous x-ui run left behind for this inbound is
// terminated — identified via the pid file we ask openvpn to write — so the
// port is guaranteed to be free. Unrelated openvpn processes are never
// touched (unlike mtg, openvpn is a system package other software may use).
func (p *Process) Start() error {
	if p.IsRunning() {
		return errors.New("openvpn is already running")
	}
	binary := GetBinaryPath()
	if binary == "" {
		return errors.New("openvpn binary not found (install openvpn or drop an 'openvpn' binary next to the x-ui binary)")
	}
	killStaleDaemon(filepath.Dir(p.configPath))
	cmd := exec.Command(binary, "--config", p.configPath)
	cmd.Stdout = p.logWriter
	cmd.Stderr = p.logWriter
	p.cmd = cmd
	p.done = make(chan struct{})
	p.exitErr = nil
	p.intentionalStop.Store(false)
	if err := cmd.Start(); err != nil {
		close(p.done)
		p.cmd = nil
		return err
	}
	go p.wait(cmd)
	return nil
}

// killStaleDaemon terminates a daemon recorded in the pid file (left behind
// by a previous x-ui run). It verifies the process still looks like openvpn
// before killing, so a recycled PID belonging to other software is spared.
func killStaleDaemon(dataDir string) {
	pidPath := filepath.Join(dataDir, "openvpn.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return
	}
	if !processLooksLikeOpenVPN(pid) {
		_ = os.Remove(pidPath)
		return
	}
	if err := signalProcess(pid, syscall.SIGTERM); err == nil {
		for i := 0; i < 20; i++ {
			if !isProcessAlive(pid) {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		if isProcessAlive(pid) {
			_ = signalProcess(pid, syscall.SIGKILL)
		}
		logger.Infof("openvpn: terminated stale daemon (pid %d) from a previous run", pid)
	}
	_ = os.Remove(pidPath)
}

// isProcessAlive is platform-specific (signal_unix.go / signal_windows.go).

func processLooksLikeOpenVPN(pid int) bool {
	if runtime.GOOS == "windows" {
		// We cannot cheaply inspect the command line on Windows, so never
		// kill from a stale pid file there — the daemon start will fail
		// visibly and the log shows why.
		return false
	}
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}
	return strings.Contains(string(cmdline), "openvpn")
}

func (p *Process) wait(cmd *exec.Cmd) {
	defer close(p.done)
	err := cmd.Wait()
	p.logWriter.Flush()
	if err == nil || p.intentionalStop.Load() {
		return
	}
	logger.Errorf("openvpn: daemon process exited: %v", err)
	p.exitErr = err
}

// Stop terminates the running daemon gracefully, falling back to a kill.
func (p *Process) Stop() error {
	if !p.IsRunning() {
		return nil
	}
	p.intentionalStop.Store(true)

	if runtime.GOOS == "windows" {
		if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		<-p.done
		return nil
	}
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case <-p.done:
	case <-time.After(gracefulStopTimeout):
	}
	if p.IsRunning() {
		_ = p.cmd.Process.Kill()
		select {
		case <-p.done:
		case <-time.After(forceStopTimeout):
		}
	}
	return nil
}

// FreeLocalPort allocates an ephemeral loopback TCP port for the daemon's
// management interface.
func FreeLocalPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
