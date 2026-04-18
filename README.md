# NTMonitor Gateway

Simple Go project that reads a JSON node list and connects to each node over SSH to return basic host information.

## Files

- `config.json` - sample node list with `ip`, `username`, `password`, and `os`.
- `main.go` - loads config, connects to SSH, and prints basic node info.

## Run

```sh
cd /home/ratnadeep/PROJECTS/NTMonitor_gateway
go run main.go
```

To use a different config file:

```sh
go run main.go custom-config.json
```

## Supported OS types

- `linux`
- `windows`

If a node is configured as `windows`, the project attempts to execute `cmd.exe` over SSH.
