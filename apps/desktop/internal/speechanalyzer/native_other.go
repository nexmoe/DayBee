//go:build !darwin || !cgo

package speechanalyzer

import "context"

func Available() bool { return false }

func request(ctx context.Context, _, _ string, _ []float32, _ int) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return Result{}, ErrUnavailable
}
