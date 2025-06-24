build:
	@go build -o bin/fs

run: build
	@./bin/fs

test:
	@go test -v ./...

clean:
	rm -rf test_tmp
	rm -rf test_output
