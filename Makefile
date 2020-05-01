default:
	go install

bootstrap:
	go get github.com/mitchellh/cli
	go get golang.org/x/oauth2
	go get github.com/google/go-github/github

dev: 
	go build -o tfteam

vet:
	go vet ./...
