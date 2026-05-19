package pprof

import (
    "fmt"
    "io"
    "net/http"
    "os"
    "time"
)

type Capturer struct {
    BaseURL string
    OutDir  string
}

// CaptureCPU fetches a CPU profile from the router's pprof endpoint.
// The router must be started with --pprof-bind=:6060.
// In this PoC, the mock router does not expose pprof — this stub
// is wired for when the real router is targeted.
func (c *Capturer) CaptureCPU(duration time.Duration) (string, error) {
    url := fmt.Sprintf("%s/debug/pprof/profile?seconds=%d",
        c.BaseURL, int(duration.Seconds()))

    resp, err := http.Get(url)
    if err != nil {
        return "", fmt.Errorf("pprof endpoint unreachable (is --pprof-bind set?): %w", err)
    }
    defer resp.Body.Close()

    outPath := fmt.Sprintf("%s/cpu-%d.pprof", c.OutDir, time.Now().Unix())
    f, err := os.Create(outPath)
    if err != nil {
        return "", err
    }
    defer f.Close()

    io.Copy(f, resp.Body)
    return outPath, nil
}
