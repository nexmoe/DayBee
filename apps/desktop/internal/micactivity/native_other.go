//go:build !darwin && !windows

package micactivity

func NativeScan() ([]Application, error) { return nil, ErrUnsupported }
