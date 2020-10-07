.PHONY: test clean

sqsjfr: *.go go.* cmd/sqsjfr/*.go
	cd cmd/sqsjfr && go build -o ../../sqsjfr .

test:
	go test -v ./...

clean:
	rm -rf sqsjfr dist/

build-releases:
	goreleaser build --snapshot --rm-dist
