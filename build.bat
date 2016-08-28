if not exist bin md bin

cd rsc
REM get the necessary dependencies that are only imported in the asset maker
go get github.com/gonutz/ld36/rsc
go run make_assets.go ..\bin\blob
cd ..

setlocal
set GODEBUG=cgocheck=0
go build -ldflags -H=windowsgui -o bin\ld36_no_data.exe
endlocal

cd bin
payload -exe=ld36_no_data.exe -data=blob -output ld36.exe
cd ..

rm bin\ld36_no_data.exe
rm bin\blob