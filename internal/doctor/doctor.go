// Package doctor performs hardware, runtime, storage, and provider probes and
// produces the hmt doctor report.
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/internal/config"
	"github.com/therealmangoosey/TAB-IGNORE/internal/disk"
	"github.com/therealmangoosey/TAB-IGNORE/internal/mux"
	"github.com/therealmangoosey/TAB-IGNORE/internal/play"
	"github.com/therealmangoosey/TAB-IGNORE/internal/provider"
	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

// Probe gathers a status snapshot for the local device.
func Probe(ctx context.Context, cfg config.Config, reg *provider.Registry, dbPath string, extra func(*hermit.Status)) (hermit.Status, error) {
	profile := DeviceProfile(cfg)
	var status hermit.Status
	status.Version = version()
	status.Profile = profile
	status.ServerAddr = cfg.Server.Addr

	reserve, _ := config.ParseSize(cfg.Disk.Reserve)
	if stats, err := disk.Info(cfg.Library.Path); err == nil {
		status.FreeBytes = stats.FreeBytes
		status.ReserveBytes = reserve
		status.SpareBytes = stats.SpareBytes()
	}

	if extra != nil {
		extra(&status)
	}

	return status, nil
}

// DeviceProfile detects runtime details and applies the plan's performanc
// profile for the target Tab A hardware.
func DeviceProfile(cfg config.Config) hermit.DeviceProfile {
	p := hermit.DeviceProfile{
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		GoVersion:   runtime.Version(),
		CPUCount:    runtime.NumCPU(),
		Termux:      isTermux(),
		FFmpeg:      mux.Available(),
		Mpv:         commandExists("mpv"),
		VLC:         commandExists("vlc"),
		StorageOK:   disk.Ensure(cfg.Library.Path) == nil,
		ProfileName: "default",
	}
	if batteryPct, charging, err := batteryStatus(); err == nil {
		p.BatteryPct = batteryPct
		p.Charging = charging
	}
	if p.Arch == "arm64" && p.OS == "linux" {
		p.ProfileName = "TabA9"
		if p.CPUCount <= 6 {
			p.ProfileName = "TabA8"
		}
	}
	p.PrivacyOK = p.StorageOK && cfg.Server.Lan == ""
	return p
}

// CommandExists reports whether a command is on PATH.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func isTermux() bool {
	if os.Getenv("PREFIX") != "" {
		return true
	}
	home := os.Getenv("HOME")
	return strings.Contains(home, "com.termux")
}

type termuxBattery struct {
	Percentage int    `json:"percentage"`
	Charging   bool   `json:"charging"`
	Health     string `json:"health"`
}

func batteryStatus() (int, bool, error) {
	if _, err := exec.LookPath("termux-battery-status"); err != nil {
		return 0, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "termux-battery-status").Output()
	if err != nil {
		return 0, false, err
	}
	var b termuxBattery
	if err := json.Unmarshal(out, &b); err != nil {
		return 0, false, err
	}
	return b.Percentage, b.Charging, nil
}

func version() string {
	if v, ok := os.LookupEnv("HERMIT_VERSION"); ok {
		return v
	}
	return "devel"
}

// FormatText renders the doctor report as a short terminal-friendly block.
func FormatText(st hermit.Status) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("hermit %s (%s/%s, %s)\n", st.Version, st.Profile.OS, st.Profile.Arch, st.Profile.GoVersion))
	b.WriteString(fmt.Sprintf("profile: %s · cpu %d · termux %v\n", st.Profile.ProfileName, st.Profile.CPUCount, st.Profile.Termux))
	if st.Profile.BatteryPct > 0 {
		b.WriteString(fmt.Sprintf("battery: %d%% (charging %v)\n", st.Profile.BatteryPct, st.Profile.Charging))
	}
	b.WriteString(fmt.Sprintf("ffmpeg: %v · mpv: %v · vlc: %v\n", st.Profile.FFmpeg, st.Profile.Mpv, st.Profile.VLC))
	b.WriteString(fmt.Sprintf("storage: ok=%v spare=%.1fG free=%.1fG\n", st.Profile.StorageOK, float64(st.SpareBytes)/1e9, float64(st.FreeBytes)/1e9))
	b.WriteString(fmt.Sprintf("player handoff: %v\n", play.Available()))
	for _, hp := range st.Providers {
		mark := "✓"
		if !hp.OK {
			mark = "✗"
		}
		b.WriteString(fmt.Sprintf("provider %s [%d %s]: %s %s\n", hp.ID, hp.LatencyMS, "ms", mark, hp.LastErr))
	}
	return b.String()
}
