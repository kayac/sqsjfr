.PHONY: test clean

sqsjfr: *.go go.* cmd/sqsjfr/*.go
	cd cmd/sqsjfr && go build -o ../../sqsjfr .

test:
	go test ./...

clean:
	rm -rf sqsjfr dist/
