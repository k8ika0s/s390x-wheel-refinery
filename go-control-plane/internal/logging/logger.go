package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

// Level represents log severity
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// contextKey is the type for context keys
type contextKey string

const (
	// CorrelationIDKey is the context key for correlation IDs
	CorrelationIDKey contextKey = "correlation_id"
	// RequestIDKey is the context key for request IDs
	RequestIDKey contextKey = "request_id"
	// UserIDKey is the context key for user IDs
	UserIDKey contextKey = "user_id"
)

// Entry represents a structured log entry
type Entry struct {
	Timestamp     time.Time      `json:"timestamp"`
	Level         Level          `json:"level"`
	Message       string         `json:"message"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	RequestID     string         `json:"request_id,omitempty"`
	UserID        string         `json:"user_id,omitempty"`
	Component     string         `json:"component,omitempty"`
	Fields        map[string]any `json:"fields,omitempty"`
	Error         string         `json:"error,omitempty"`
	Stack         string         `json:"stack,omitempty"`
}

// Logger provides structured logging with context support
type Logger struct {
	output    io.Writer
	minLevel  Level
	component string
}

// New creates a new structured logger
func New(component string) *Logger {
	return &Logger{
		output:    os.Stdout,
		minLevel:  LevelInfo,
		component: component,
	}
}

// NewWithWriter creates a logger with custom output
func NewWithWriter(component string, w io.Writer) *Logger {
	return &Logger{
		output:    w,
		minLevel:  LevelInfo,
		component: component,
	}
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level Level) {
	l.minLevel = level
}

// WithContext creates a logger with context values
func (l *Logger) WithContext(ctx context.Context) *ContextLogger {
	return &ContextLogger{
		logger: l,
		ctx:    ctx,
	}
}

// WithFields creates a logger with additional fields
func (l *Logger) WithFields(fields map[string]any) *FieldLogger {
	return &FieldLogger{
		logger: l,
		fields: fields,
	}
}

// log writes a structured log entry
func (l *Logger) log(level Level, msg string, fields map[string]any, err error) {
	if !l.shouldLog(level) {
		return
	}

	entry := Entry{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Message:   msg,
		Component: l.component,
		Fields:    fields,
	}

	if err != nil {
		entry.Error = err.Error()
	}

	data, jsonErr := json.Marshal(entry)
	if jsonErr != nil {
		// Fallback to standard logging if JSON marshaling fails
		log.Printf("ERROR: failed to marshal log entry: %v", jsonErr)
		log.Printf("%s [%s] %s: %s", entry.Timestamp.Format(time.RFC3339), level, l.component, msg)
		return
	}

	fmt.Fprintln(l.output, string(data))
}

// shouldLog checks if the level should be logged
func (l *Logger) shouldLog(level Level) bool {
	levels := map[Level]int{
		LevelDebug: 0,
		LevelInfo:  1,
		LevelWarn:  2,
		LevelError: 3,
	}
	return levels[level] >= levels[l.minLevel]
}

// Debug logs a debug message
func (l *Logger) Debug(msg string) {
	l.log(LevelDebug, msg, nil, nil)
}

// Info logs an info message
func (l *Logger) Info(msg string) {
	l.log(LevelInfo, msg, nil, nil)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string) {
	l.log(LevelWarn, msg, nil, nil)
}

// Error logs an error message
func (l *Logger) Error(msg string, err error) {
	l.log(LevelError, msg, nil, err)
}

// ContextLogger wraps a logger with context
type ContextLogger struct {
	logger *Logger
	ctx    context.Context
}

// log writes a log entry with context values
func (cl *ContextLogger) log(level Level, msg string, fields map[string]any, err error) {
	if !cl.logger.shouldLog(level) {
		return
	}

	entry := Entry{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Message:   msg,
		Component: cl.logger.component,
		Fields:    fields,
	}

	// Extract context values
	if corrID, ok := cl.ctx.Value(CorrelationIDKey).(string); ok {
		entry.CorrelationID = corrID
	}
	if reqID, ok := cl.ctx.Value(RequestIDKey).(string); ok {
		entry.RequestID = reqID
	}
	if userID, ok := cl.ctx.Value(UserIDKey).(string); ok {
		entry.UserID = userID
	}

	if err != nil {
		entry.Error = err.Error()
	}

	data, jsonErr := json.Marshal(entry)
	if jsonErr != nil {
		log.Printf("ERROR: failed to marshal log entry: %v", jsonErr)
		return
	}

	fmt.Fprintln(cl.logger.output, string(data))
}

// Debug logs a debug message with context
func (cl *ContextLogger) Debug(msg string) {
	cl.log(LevelDebug, msg, nil, nil)
}

// Info logs an info message with context
func (cl *ContextLogger) Info(msg string) {
	cl.log(LevelInfo, msg, nil, nil)
}

// Warn logs a warning message with context
func (cl *ContextLogger) Warn(msg string) {
	cl.log(LevelWarn, msg, nil, nil)
}

// Error logs an error message with context
func (cl *ContextLogger) Error(msg string, err error) {
	cl.log(LevelError, msg, nil, err)
}

// WithFields adds fields to the context logger
func (cl *ContextLogger) WithFields(fields map[string]any) *ContextFieldLogger {
	return &ContextFieldLogger{
		logger: cl.logger,
		ctx:    cl.ctx,
		fields: fields,
	}
}

// FieldLogger wraps a logger with fields
type FieldLogger struct {
	logger *Logger
	fields map[string]any
}

// Debug logs a debug message with fields
func (fl *FieldLogger) Debug(msg string) {
	fl.logger.log(LevelDebug, msg, fl.fields, nil)
}

// Info logs an info message with fields
func (fl *FieldLogger) Info(msg string) {
	fl.logger.log(LevelInfo, msg, fl.fields, nil)
}

// Warn logs a warning message with fields
func (fl *FieldLogger) Warn(msg string) {
	fl.logger.log(LevelWarn, msg, fl.fields, nil)
}

// Error logs an error message with fields
func (fl *FieldLogger) Error(msg string, err error) {
	fl.logger.log(LevelError, msg, fl.fields, err)
}

// ContextFieldLogger combines context and fields
type ContextFieldLogger struct {
	logger *Logger
	ctx    context.Context
	fields map[string]any
}

// log writes a log entry with both context and fields
func (cfl *ContextFieldLogger) log(level Level, msg string, err error) {
	if !cfl.logger.shouldLog(level) {
		return
	}

	entry := Entry{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Message:   msg,
		Component: cfl.logger.component,
		Fields:    cfl.fields,
	}

	// Extract context values
	if corrID, ok := cfl.ctx.Value(CorrelationIDKey).(string); ok {
		entry.CorrelationID = corrID
	}
	if reqID, ok := cfl.ctx.Value(RequestIDKey).(string); ok {
		entry.RequestID = reqID
	}
	if userID, ok := cfl.ctx.Value(UserIDKey).(string); ok {
		entry.UserID = userID
	}

	if err != nil {
		entry.Error = err.Error()
	}

	data, jsonErr := json.Marshal(entry)
	if jsonErr != nil {
		log.Printf("ERROR: failed to marshal log entry: %v", jsonErr)
		return
	}

	fmt.Fprintln(cfl.logger.output, string(data))
}

// Debug logs a debug message with context and fields
func (cfl *ContextFieldLogger) Debug(msg string) {
	cfl.log(LevelDebug, msg, nil)
}

// Info logs an info message with context and fields
func (cfl *ContextFieldLogger) Info(msg string) {
	cfl.log(LevelInfo, msg, nil)
}

// Warn logs a warning message with context and fields
func (cfl *ContextFieldLogger) Warn(msg string) {
	cfl.log(LevelWarn, msg, nil)
}

// Error logs an error message with context and fields
func (cfl *ContextFieldLogger) Error(msg string, err error) {
	cfl.log(LevelError, msg, err)
}

// Helper functions for context management

// WithCorrelationID adds a correlation ID to the context
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, id)
}

// WithRequestID adds a request ID to the context
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, RequestIDKey, id)
}

// WithUserID adds a user ID to the context
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, UserIDKey, id)
}

// GetCorrelationID retrieves the correlation ID from context
func GetCorrelationID(ctx context.Context) string {
	if id, ok := ctx.Value(CorrelationIDKey).(string); ok {
		return id
	}
	return ""
}

// GetRequestID retrieves the request ID from context
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// Made with Bob
