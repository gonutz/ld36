if not exist bin md bin

cd rsc
go run make_assets.go ..\bin\blob
cd ..

go build -ldflags -H=windowsgui -o bin\ld36_no_data.exe

cd bin
payload -exe=ld36_no_data.exe -data=blob -output ld36.exe
cd ..

rm bin\ld36_no_data.exe
rm bin\blob