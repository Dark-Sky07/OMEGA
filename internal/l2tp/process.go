package l2tp

import (
	"errors"
	"fmt"
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

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

var (
	gracefulStopTimeout = 7 * time.Second
	forceStopTimeout    = 3 * time.Second
)

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
		line := strings.TrimSpace(strings.TrimRight(w.buf[:i], "\r"))
		w.buf = w.buf[i+1:]
		if line != "" {
			w.lastLine = line
			logger.Infof("l2tp: %s | %s", w.label, line)
		}
	}
	return len(p), nil
}

func (w *procLogWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	line := strings.TrimSpace(strings.TrimRight(w.buf, "\r"))
	if line != "" {
		w.lastLine = line
		logger.Infof("l2tp: %s | %s", w.label, line)
	}
	w.buf = ""
}

func (w *procLogWriter) LastLine() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastLine
}

// Process supervises the strongSwan starter and xl2tpd foreground processes.
type Process struct {
	id          int
	configDir   string
	ipsecCmd    *exec.Cmd
	xl2tpdCmd   *exec.Cmd
	ipsecDone   chan struct{}
	xl2tpdDone  chan struct{}
	ipsecLog    *procLogWriter
	xl2tpdLog   *procLogWriter
	exitErr     error
	intentional atomic.Bool
	mu          sync.Mutex
}

func newProcess(id int) *Process {
	return &Process{
		id:        id,
		configDir: dataDirForID(id),
		ipsecLog:  &procLogWriter{label: "strongSwan"},
		xl2tpdLog: &procLogWriter{label: "xl2tpd"},
	}
}

// processEnvironment replaces inherited values instead of appending duplicate
// entries. getenv(3) uses the first matching entry, so appending a per-inbound
// runtime override after an inherited value can silently select the wrong
// configuration.
func processEnvironment(overrides ...string) []string {
	overrideKeys := make(map[string]struct{}, len(overrides))
	for _, entry := range overrides {
		if key, _, ok := strings.Cut(entry, "="); ok {
			overrideKeys[key] = struct{}{}
		}
	}
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, overridden := overrideKeys[key]; overridden {
				continue
			}
		}
		env = append(env, entry)
	}
	return append(env, overrides...)
}

func strongSwanPath() string {
	for _, name := range []string{"ipsec", "strongswan"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func xl2tpdPath() string {
	path, _ := exec.LookPath("xl2tpd")
	return path
}

func compiledStrongSwanPIDDir() string {
	ipsec := strongSwanPath()
	if ipsec == "" {
		return ""
	}
	output, err := exec.Command(ipsec, "--piddir").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func stopPIDFile(path, expected string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 1 {
		return
	}
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || !strings.Contains(strings.ToLower(string(cmdline)), expected) {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = process.Signal(syscall.SIGTERM)
	deadline := time.Now().Add(gracefulStopTimeout)
	for time.Now().Before(deadline) {
		if err := process.Signal(syscall.Signal(0)); err != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = process.Kill()
}

// stopOrphan stops a daemon group left behind by a previous panel process.
// The manager is in-memory, so PID/config discovery is required after a host
// or panel restart; without it an old xl2tpd would keep UDP 1701 occupied and
// the old strongSwan starter would own the IKE sockets.
func stopOrphan(id int) {
	if runtime.GOOS == "windows" {
		return
	}
	stopPIDFile(xl2tpdPIDPath(id), "xl2tpd")
	// The distro ipsec wrapper overwrites IPSEC_PIDDIR, so its stop command
	// cannot address the per-inbound path used by the old implementation. Ask
	// the wrapper for its compiled PID directory and stop only processes whose
	// command lines are actually starter/charon; this also reclaims a daemon
	// left behind after the panel itself was restarted.
	if pidDir := compiledStrongSwanPIDDir(); pidDir != "" {
		stopPIDFile(filepath.Join(pidDir, "starter.charon.pid"), "starter")
		stopPIDFile(filepath.Join(pidDir, "charon.pid"), "charon")
	}
}

// Available reports whether both daemon entry points are installed.
func Available() bool {
	return strongSwanPath() != "" && xl2tpdPath() != ""
}

func (p *Process) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return commandRunning(p.ipsecCmd, p.ipsecDone) && commandRunning(p.xl2tpdCmd, p.xl2tpdDone)
}

func commandRunning(cmd *exec.Cmd, done chan struct{}) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}
	// ProcessState is written by Cmd.Wait from the waiter goroutine. Rely on
	// the closed done channel instead of reading it here, so IsRunning never
	// races with that write (a closed channel also provides the required
	// happens-before edge).
	if done == nil {
		return true
	}
	select {
	case <-done:
		return false
	default:
		return true
	}
}

func (p *Process) GetResult() string {
	if line := p.xl2tpdLog.LastLine(); line != "" {
		return line
	}
	if line := p.ipsecLog.LastLine(); line != "" {
		return line
	}
	p.mu.Lock()
	err := p.exitErr
	p.mu.Unlock()
	if err != nil {
		return err.Error()
	}
	return ""
}

// ensureControlFIFO creates the control FIFO explicitly before xl2tpd starts.
// Several distro packages do not install /var/run/xl2tpd or its default
// l2tp-control FIFO when the systemd unit is disabled. xl2tpd then exits with
// "open_controlfd: Unable to open /var/run/xl2tpd/l2tp-control", even when the
// rest of the generated configuration is valid. Keeping a private FIFO beside
// the per-inbound config also avoids a global control-path collision.
func ensureControlFIFO(id int) error {
	path := xl2tpdControlPath(id)
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeNamedPipe == 0 {
			return fmt.Errorf("xl2tpd control path is not a FIFO: %s", path)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("chmod xl2tpd control FIFO: %w", err)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect xl2tpd control FIFO: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create xl2tpd control directory: %w", err)
	}
	// Use the system mkfifo utility rather than syscall.Mkfifo so this package
	// continues to compile for the Windows release target as well.
	if output, err := exec.Command("mkfifo", "-m", "600", path).CombinedOutput(); err != nil {
		return fmt.Errorf("create xl2tpd control FIFO: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (p *Process) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if commandRunning(p.ipsecCmd, p.ipsecDone) || commandRunning(p.xl2tpdCmd, p.xl2tpdDone) {
		return errors.New("l2tp daemons are already running")
	}
	ipsec := strongSwanPath()
	xl2tpd := xl2tpdPath()
	if ipsec == "" {
		return errors.New("strongSwan ipsec command not found")
	}
	if xl2tpd == "" {
		return errors.New("xl2tpd command not found")
	}

	// The distro ipsec wrapper resets IPSEC_CONFDIR to its compiled /etc path,
	// so pass the managed connection explicitly to starter. STRONGSWAN_CONF
	// points charon at the AppArmor-readable per-inbound config under /etc and
	// that config redirects the legacy stroke secrets loader to the PSK file.
	ipsecCmd := exec.Command(ipsec, "start", "--nofork", "--conf", ipsecConfigPath(p.id))
	ipsecCmd.Env = processEnvironment(
		"STRONGSWAN_CONF=" + strongSwanConfigPath(p.id),
	)
	ipsecCmd.Stdout = p.ipsecLog
	ipsecCmd.Stderr = p.ipsecLog
	if err := ipsecCmd.Start(); err != nil {
		return fmt.Errorf("start strongSwan: %w", err)
	}
	p.ipsecCmd = ipsecCmd
	p.ipsecDone = make(chan struct{})
	p.exitErr = nil
	p.intentional.Store(false)
	go p.wait(ipsecCmd, p.ipsecDone, p.ipsecLog, "strongSwan")

	if err := ensureControlFIFO(p.id); err != nil {
		p.intentional.Store(true)
		_ = ipsecCmd.Process.Signal(syscall.SIGTERM)
		return err
	}
	xl2tpdCmd := exec.Command(xl2tpd, "-D", "-c", xl2tpdConfigPath(p.id), "-C", xl2tpdControlPath(p.id), "-p", xl2tpdPIDPath(p.id))
	xl2tpdCmd.Stdout = p.xl2tpdLog
	xl2tpdCmd.Stderr = p.xl2tpdLog
	if err := xl2tpdCmd.Start(); err != nil {
		p.intentional.Store(true)
		_ = ipsecCmd.Process.Signal(syscall.SIGTERM)
		return fmt.Errorf("start xl2tpd: %w", err)
	}
	p.xl2tpdCmd = xl2tpdCmd
	p.xl2tpdDone = make(chan struct{})
	go p.wait(xl2tpdCmd, p.xl2tpdDone, p.xl2tpdLog, "xl2tpd")

	// Start() must not report success merely because both child processes were
	// forked. In particular, charon can reject its configuration immediately
	// while xl2tpd continues listening on UDP 1701. Treat an early exit as a
	// failed start so the manager never advertises a half-alive daemon group.
	if err := p.waitForStartup(2 * time.Second); err != nil {
		p.intentional.Store(true)
		_ = xl2tpdCmd.Process.Signal(syscall.SIGTERM)
		_ = ipsecCmd.Process.Signal(syscall.SIGTERM)
		return err
	}
	return nil
}

func (p *Process) waitForStartup(timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-p.ipsecDone:
			line := p.ipsecLog.LastLine()
			if line == "" {
				line = "process exited during startup"
			}
			return fmt.Errorf("strongSwan failed during startup: %s", line)
		case <-p.xl2tpdDone:
			line := p.xl2tpdLog.LastLine()
			if line == "" {
				line = "process exited during startup"
			}
			return fmt.Errorf("xl2tpd failed during startup: %s", line)
		case <-timer.C:
			return nil
		}
	}
}

func (p *Process) wait(cmd *exec.Cmd, done chan struct{}, writer *procLogWriter, name string) {
	err := cmd.Wait()
	writer.Flush()
	// Publish process termination before taking p.mu. Start may be waiting for
	// this channel while it still owns the mutex during its startup handshake.
	close(done)
	if err != nil && !p.intentional.Load() {
		p.mu.Lock()
		p.exitErr = fmt.Errorf("%s exited: %w", name, err)
		p.mu.Unlock()
		logger.Errorf("l2tp: %s exited: %v", name, err)
	}
}

func (p *Process) Stop() error {
	p.mu.Lock()
	p.intentional.Store(true)
	ipsecCmd, ipsecDone := p.ipsecCmd, p.ipsecDone
	xl2tpdCmd, xl2tpdDone := p.xl2tpdCmd, p.xl2tpdDone
	p.mu.Unlock()

	if runtime.GOOS == "windows" {
		if xl2tpdCmd != nil && xl2tpdCmd.Process != nil {
			_ = xl2tpdCmd.Process.Kill()
		}
		if ipsecCmd != nil && ipsecCmd.Process != nil {
			_ = ipsecCmd.Process.Kill()
		}
		return nil
	}
	if xl2tpdCmd != nil && xl2tpdCmd.Process != nil {
		_ = xl2tpdCmd.Process.Signal(syscall.SIGTERM)
	}
	if ipsecCmd != nil && ipsecCmd.Process != nil {
		_ = ipsecCmd.Process.Signal(syscall.SIGTERM)
	}
	waitForDone(xl2tpdDone, gracefulStopTimeout)
	waitForDone(ipsecDone, gracefulStopTimeout)

	p.mu.Lock()
	stillXL2TPD := commandRunning(p.xl2tpdCmd, p.xl2tpdDone)
	stillIPsec := commandRunning(p.ipsecCmd, p.ipsecDone)
	if stillXL2TPD && p.xl2tpdCmd != nil && p.xl2tpdCmd.Process != nil {
		_ = p.xl2tpdCmd.Process.Kill()
	}
	if stillIPsec && p.ipsecCmd != nil && p.ipsecCmd.Process != nil {
		_ = p.ipsecCmd.Process.Kill()
	}
	p.mu.Unlock()
	waitForDone(xl2tpdDone, forceStopTimeout)
	waitForDone(ipsecDone, forceStopTimeout)
	return nil
}

func waitForDone(done chan struct{}, timeout time.Duration) {
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(timeout):
	}
}
