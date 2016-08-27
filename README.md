This is my entry for the [Ludum Dare 36](http://ludumdare.com/compo/) Compo.

Right now this project is Windows only. 

To build the project you need to have [Go](https://golang.org/dl/) and a [C-compiler](https://sourceforge.net/projects/mingw/files/latest/download?source=files) installed. You can run

`go get github.com/gonutz/ld36`

to get the source code. Go to `%GOPATH%\src\github.com\gonutz\ld36` and run `build.bat`. This should get the necessary dependencies, create the game assets from the rsc folder and build the game into the (newly created) bin folder.

Run `bin\ld36.exe`.