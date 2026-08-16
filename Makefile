.PHONY: build test up down restart logs watch clean

# Build both binaries locally (host use / CI).
build:
	go build -o bin/termupd ./cmd/termupd
	go build -o bin/termup ./cmd/termup

# Run the full test suite with the race detector.
test:
	go test ./... -race

# The live target list is not tracked; seed it from the template on first run.
# (compose bind-mounts ./config.yaml, and a missing path would be created as a
# directory instead of failing usefully.)
config.yaml:
	cp config.example.yaml config.yaml
	@echo "created config.yaml from config.example.yaml — edit it, then re-run"

# Build the image and run the daemon continuously (detached, auto-restart).
up: config.yaml
	docker compose up -d --build

# Stop and remove the container (keeps the db volume; add -v to wipe it).
down:
	docker compose down

# Reload after editing config.yaml (or any restart).
restart:
	docker compose restart termupd

# Follow the daemon logs (probes, state changes, alerts).
logs:
	docker compose logs -f termupd

# Attach the read-only dashboard to the running daemon (inside the container).
watch:
	docker compose exec termupd termup watch

# Remove local build artifacts and the dev database.
clean:
	rm -rf bin termup.db termup.db-journal
