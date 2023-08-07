# gohlslib

[![Test](https://github.com/bluenviron/gohlslib/workflows/test/badge.svg)](https://github.com/bluenviron/gohlslib/actions?query=workflow:test)
[![Lint](https://github.com/bluenviron/gohlslib/workflows/lint/badge.svg)](https://github.com/bluenviron/gohlslib/actions?query=workflow:lint)
[![Go Report Card](https://goreportcard.com/badge/github.com/bluenviron/gohlslib)](https://goreportcard.com/report/github.com/bluenviron/gohlslib)
[![CodeCov](https://codecov.io/gh/bluenviron/gohlslib/branch/main/graph/badge.svg)](https://app.codecov.io/gh/bluenviron/gohlslib/branch/main)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/bluenviron/gohlslib)](https://pkg.go.dev/github.com/bluenviron/gohlslib#pkg-index)

HLS client and muxer library for the Go programming language, written for [MediaMTX](https://github.com/bluenviron/mediamtx).

Go &ge; 1.18 is required.

Features:

* Client

  * Read MPEG-TS streams
  * Read fMP4 streams
  * Read AV1 tracks
  * Read VP9 tracks
  * Read H265 tracks
  * Read H264 tracks
  * Read Opus tracks
  * Read MPEG-4 audio (AAC) tracks

* Muxer

  * Generate MPEG-TS streams
  * Generate fMP4 streams
  * Generate Low-latency streams
  * Write AV1 tracks
  * Write VP9 tracks
  * Write H265 tracks
  * Write H264 tracks
  * Write Opus tracks
  * Write MPEG-4 audio (AAC) tracks
  * Save generated segments on disk

* General

  * Parse and produce M3U8 playlists
  * Examples

## Table of contents

* [Examples](#examples)
* [API Documentation](#api-documentation)
* [Standards](#standards)
* [Related projects](#related-projects)

## Examples

* [playlist-parser](examples/playlist-parser/main.go)
* [client](examples/client/main.go)
* [muxer](examples/muxer/main.go)

## API Documentation

https://pkg.go.dev/github.com/bluenviron/gohlslib#pkg-index

## Standards

* [RFC2616, HTTP 1.1](https://datatracker.ietf.org/doc/html/rfc2616)
* [RFC8216, HLS](https://datatracker.ietf.org/doc/html/rfc8216)
* [HLS v2](https://datatracker.ietf.org/doc/html/draft-pantos-hls-rfc8216bis)
* [Codec standards](https://github.com/bluenviron/mediacommon#standards)
* [Golang project layout](https://github.com/golang-standards/project-layout)

## Related projects

* [MediaMTX](https://github.com/bluenviron/mediamtx)
* [gortsplib](https://github.com/bluenviron/gortsplib)
* [mediacommon](https://github.com/bluenviron/mediacommon)
