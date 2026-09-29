set GOROOT=%PROGRAMFILES%\Go
set GOARCH=amd64
set GOOS=linux
set CGO_ENABLED=0
set GO111MODULE=auto
go build -ldflags="-s -w" -trimpath -o  ai-cleaner