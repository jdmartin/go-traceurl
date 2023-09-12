[![CodeQL](https://github.com/jdmartin/go-traceurl/actions/workflows/codeql.yml/badge.svg)](https://github.com/jdmartin/go-traceurl/actions/workflows/codeql.yml)
[![Docker](https://github.com/jdmartin/go-traceurl/actions/workflows/docker_build.yml/badge.svg)](https://github.com/jdmartin/go-traceurl/actions/workflows/docker_build.yml)

# go-traceurl
A Go implementation of a URL tracer.

------

🚨 I've only been using Go since late May 2023. 🚨

[wheregoes.com](https://wheregoes.com) remains a fine site that you should probably use instead.  

I'm just doing this to learn Go.

## Usage Notes
There are some env variables that can be set:

- SERVE: set to 'tcp' to serve on PORT (see below), or 'socket' to serve on /tmp/go-trace.sock
- PORT: The port for the tcp server to listen on. Defaults to 8080
- MODE: [Currently in development] 
