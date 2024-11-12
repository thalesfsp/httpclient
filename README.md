# httpclient

A robust and feature-rich Go HTTP client library that provides pre-configured HTTP clients with built-in retry logic, metrics tracking, and comprehensive logging capabilities.

## Features

- **Pre-configured HTTP Client**: Ready-to-use client with sensible defaults
- **Retry Logic**: Built-in exponential backoff retry mechanism with customizable settings
- **Metrics Tracking**: Integrated counters for monitoring request success, failures, and retries
- **Structured Logging**: Comprehensive logging of requests, responses, and errors
- **Header Management**: Support for both default and per-request headers
- **Query Parameter Handling**: Flexible configuration of query parameters
- **Custom Retry Rules**: Configurable retry patterns using regular expressions
- **Timeout Control**: Customizable timeouts for HTTP requests
- **Singleton Pattern**: Optional singleton instance for application-wide use
- **Type Safety**: Interface-based design with compile-time type checking
- **Error Handling**: Rich error context with response details and body content

## Install

### Package

```bash
go get github.com/thalesfsp/httpclient/v2
```

## Usage

See `httpclient_test.go` and other `*_test.go` files.

## Error Handling

The library uses the [customerror](http://github.com/thalesfsp/customerror) package for rich error context. Errors include:

- Request URL
- Response status code
- Response body content
- Original error message

## Retry Mechanism

The client includes built-in retry logic with:
- Exponential backoff
- Configurable retry attempts
- Default retry on:
  - `408` Request Timeout
  - `429` Too Many Requests
  - `5xx` Server Errors
- Custom retry patterns via regex

## Contributing

1. Fork the repository
2. Clone your fork
3. Create a feature branch
4. Make your changes
5. Run tests
6. Create a pull request