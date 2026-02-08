.PHONY: build clean stop start restart test

build:
	go build -o news-app ./cmd/news-app

clean:
	rm -f news-app

test:
	go test ./...
