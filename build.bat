if not exist bin md bin

cd rsc
REM get the necessary dependencies that are only imported in the asset maker
go get github.com/gonutz/ld36/rsc
go run make_assets.go ..\bin\blob
cd ..

go get github.com/akavel/rsrc
rsrc -arch 386 -ico icon.ico -o rsrc_386.syso
rsrc -arch amd64 -ico icon.ico -o rsrc_amd64.syso

go get github.com/gonutz/payload/cmd/payload

set GOARCH=386
go build -ldflags "-s -w -H=windowsgui" -o bin\ld36_no_data.exe

cd bin
payload -exe=ld36_no_data.exe -data=blob -output ld36.exe
cd ..

del bin\ld36_no_data.exe
del bin\blob