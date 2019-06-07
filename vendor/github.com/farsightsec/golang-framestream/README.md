# Frame Streams implementation in Go

https://github.com/farsightsec/golang-framestream

Frame Streams is a lightweight, binary-clean protocol that allows
for the transport of arbitrarily encoded data payload sequences with
minimal framing overhead.

This package provides a pure Golang implementation. The Frame Streams
implementation in C is at https://github.com/farsightsec/fstrm/.

The example framestream_dump program reads a Frame Streams formatted
input file and prints the data frames and frame byte counts.
