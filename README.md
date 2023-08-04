Wire
---
send/recieve files over ethernet with zero configuration via IPv6

Install
---
```
git clone https://github.com/archeoid/wire.git
cd wire
go get
go build
./wire i
```

Usage
---
```
wire r PATH
    start a receive session in PATH or PWD if no PATH
wire s ARGS
    send the files/folders in ARGS, can include patterns
wire wr OR wire ws
    wireless send/receive mode
wire i
    install wire
    on windows this installs into %APPDATA%\Local\Programs
        + creates explorer context menus (access with shift + right click)
    on linux this intalls into ~/.local/bin
wire u
    uninstall wire
wire h
    show help
```
