.PHONY: tidy build run test vet lint migrate keys docker

# Resolve and pin dependencies.
tidy:
	go mod tidy

# Compile the server binary.
build:
	go build -o bin/platform-console ./cmd/server

# Run the server (expects PLATFORM_* env vars; see .env.example).
run:
	go run ./cmd/server

# Run all tests.
test:
	go test ./...

# Static checks.
vet:
	go vet ./...

# Apply migrations with the Flyway CLI (adjust URL/creds as needed).
migrate:
	flyway -url=jdbc:postgresql://localhost:5432/clario360 \
	       -user=postgres -password=postgres -locations=filesystem:./migrations migrate

# Generate a dev RSA keypair for local JWT verification.
keys:
	mkdir -p .keys
	openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out .keys/jwt-private.pem
	openssl rsa -in .keys/jwt-private.pem -pubout -out .keys/jwt-public.pem
	@echo "Wrote .keys/jwt-private.pem and .keys/jwt-public.pem"

# Build the container image.
docker:
	docker build -t clario360/platform-console:dev .
