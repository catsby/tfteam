default:
	go install

bootstrap:
	go get -u github.com/mitchellh/cli
	go get -u golang.org/x/oauth2
	go get -u github.com/google/go-github/github

dev: 
	go build -o tfteam

vet:
	go vet ./...
