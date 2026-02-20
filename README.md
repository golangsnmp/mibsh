# mibsh

Interactive terminal MIB browser and SNMP query tool.

Browse OID trees, inspect object definitions, and run SNMP queries from a
single TUI.

## Install

```
go install github.com/golangsnmp/mibsh@latest
```

Pre-built binaries are available on the
[releases page](https://github.com/golangsnmp/mibsh/releases).

## MIB sources

By default mibsh searches standard system MIB locations used by net-snmp and
libsmi:

- `/usr/share/snmp/mibs`, `~/.snmp/mibs` (net-snmp)
- `/usr/share/mibs/*` (libsmi)
- `$MIBDIRS`, `$SMIPATH` environment variables
- `/etc/snmp/snmp.conf`, `~/.snmp/snmp.conf`, `/etc/smi.conf`, `~/.smirc`

If no MIBs are found on your system, install them. On Debian/Ubuntu:

```
sudo apt install snmp-mibs-downloader
```

To use custom MIB directories, pass `-p` (searched recursively):

```
mibsh -p /path/to/vendor/mibs -p /path/to/other/mibs
```

## Usage

```
mibsh [options] [MODULE ...]
```

Positional arguments are module names to load. If none are given, all modules
found in the search paths are loaded.

### Options

| Flag | Description |
|------|-------------|
| `-p PATH` | MIB search path (repeatable, recursive) |
| `-permissive` | Use permissive strictness when loading |
| `-target HOST[:PORT]` | SNMP target for queries |
| `-community STRING` | SNMP community string (default `public`) |
| `-version VERSION` | SNMP version: `1`, `2c`, `3` (default `2c`) |

### Examples

```
mibsh                              # browse all system MIBs
mibsh IF-MIB SNMPv2-MIB           # load specific modules
mibsh -p ./vendor-mibs             # custom MIB directory
mibsh -target 192.168.1.1 IF-MIB  # browse and query a device
```

## Key bindings

Press `?` inside mibsh for the full help overlay.

### Navigation

| Key | Action |
|-----|--------|
| `j`/`k`, arrows | Move up/down |
| `enter`/`l`, `h` | Expand/collapse node |
| `ctrl+d`/`ctrl+u` | Page down/up |
| `J`/`K` | Scroll detail pane |
| `backspace` | Back |

### Browsing

| Key | Action |
|-----|--------|
| `/` | Search |
| `f` | CEL filter |
| `y` | Copy OID to clipboard |
| `x` | Cross-references |

### Chords

Multi-key commands start with a prefix key. A popup shows available actions.

| Chord | Action |
|-------|--------|
| `s` + `g` | SNMP GET |
| `s` + `n` | SNMP GETNEXT |
| `s` + `w` | SNMP WALK |
| `s` + `t` | SNMP table fetch |
| `c` + `c` | Connect to device |
| `c` + `d` | Disconnect |
| `v` + `m` | Module browser |
| `v` + `y` | Type browser |
| `v` + `d` | Diagnostics |
| `v` + `r` | Results pane |

## Device profiles

Connection settings can be saved as named profiles for quick reconnection.
Profiles are stored in `~/.config/mibsh/profiles.json` (or `$XDG_CONFIG_HOME`).

Use `c` `s` to save the current connection as a profile, and `c` `c` to pick
from saved profiles when connecting.

## License

MIT
