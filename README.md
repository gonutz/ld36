This is my entry for the [Ludum Dare 36](http://ludumdare.com/compo/ludum-dare-36/?action=preview&uid=110557) Compo (2016).

Right now this project is Windows only. 

# Build

To build the project you need to have [the Go programming language](https://golang.org/dl/) installed. You also need [Git](https://git-scm.com/downloads). To build and run the program, type this in the command line:

```
go get -u github.com/gonutz/ld36
cd %GOPATH%\src\github.com\gonutz\ld36
build.bat
bin\ld36.exe
```

This will get the source code and its dependencies, then call the `build.bat` script which will generate the game's final resources, build the game and pack both into a single executable without external dependencies. The executable is in `bin\ld36.exe`. You can run this program on any Windows machine from Windows XP up.