package elements

import (
	"log"
)

// LoggingElement implements logging for transport operations
type LoggingElement struct {
	logger *log.Logger
}

func NewLoggingElement(logger *log.Logger) *LoggingElement {
	return &LoggingElement{
		logger: logger,
	}
}

func (l *LoggingElement) ProcessSend(data []byte) ([]byte, error) {
	l.logger.Printf("Sending data of length %d", len(data))
	return data, nil
}

func (l *LoggingElement) ProcessReceive(data []byte) ([]byte, error) {
	l.logger.Printf("Received data of length %d", len(data))
	return data, nil
}

func (l *LoggingElement) Name() string {
	return "logging"
}
