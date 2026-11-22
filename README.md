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
- HOST: The host ip for the tcp server to listen on. Defaults to 127.0.0.1
- SOCKET_PATH: The location of the unix socket (default /tmp/go-trace.sock) used by SERVE
- TRUST_PROXY: set to 'true' if go-trace sits behind a reverse proxy that appends the client's address to X-Forwarded-For (nginx `$proxy_add_x_forwarded_for`, Caddy, and Traefik all do), so rate limiting is per visitor instead of per proxy. Set to 'false' to always use the connection's own address. Defaults to true when SERVE=socket and false when SERVE=tcp. Only turn it on if the proxy is the only way to reach the app.
- MODE: [Currently in development] 

## Misc

There's a cli version (beta) of this tool here: [go-traceurl-cli](https://github.com/jdmartin/go-traceurl-cli)
