package logging

import (
	"log/slog"
	"os"
)

func NewLogger(level string) (*slog.Logger, error) {
	var programLevel slog.Level
	if err := programLevel.UnmarshalText([]byte(level)); err != nil {
		return nil, err
	}
	options := &slog.HandlerOptions{Level: programLevel}
	return slog.New(slog.NewJSONHandler(os.Stdout, options)), nil
}
