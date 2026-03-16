# stashpak
Build a Portable package for Arch Linux

# Usage
```bash
stashpak [action] (...arguments)
```

## Actions
### Validate
Validate takes one argument: the path to a package configuration. It parses and reports any decode or logical error.

# Dependencies

- systemd
	- run0
- GNU coreutils
- Pacman

# Environment Variables

- stashPakElevateProgram
	- Controls which program is used to elevate permissions. Defaults to run0.