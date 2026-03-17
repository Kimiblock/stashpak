# stashpak
Build a Portable package for Arch Linux

# Usage
```bash
stashpak [action] (...arguments)
```

## Actions
### validate
Validate takes one or more argument: the path to a package configuration. It parses and reports any decode or logical error.\
### install-local
Takes no argument and works in the current directory to build and install a Portable Arch package.

A configuration file named `stashpak.toml` must exist.


# Dependencies

- systemd
	- run0
- GNU coreutils
- Pacman
- Devtools
- devtools-cn-git (Optional)
- Git

# Environment Variables

- stashPakElevateProgram
	- Controls which program is used to elevate permissions. Defaults to run0.