GREEN = \033[0;32m
BLUE = \033[0;34m
RED = \033[0;31m
NC = \033[0m

all: build

prepare:
	@echo -e ":: $(GREEN) Preparing environment...$(NC)"
	@echo -e ":: $(GREEN) Downloading go dependencies...$(NC)"
	@go mod download \
		&& echo -e "==> $(BLUE) Successfully downloaded go dependencies$(NC)" \
		|| (echo -e "==> $(RED) Failed to download go dependencies$(NC)" && exit 1)

run:
	@echo -e ":: $(GREEN)Starting backend...$(NC)"
	@go build -o bin/backend cmd/backend/main.go && \
		DEBUG=true ./bin/backend \
		&& echo -e "==> $(BLUE)Successfully shut down backend$(NC)" \
		|| (echo -e "==> $(RED)Backend failed to start $(NC)" && exit 1)

build:
	@echo -e ":: $(GREEN)Building backend...$(NC)"
	@echo -e "  -> Building backend binary..."
	@go build -o bin/backend cmd/backend/main.go && echo -e "==> $(BLUE)Build completed successfully$(NC)" || (echo -e "==> $(RED)Build failed$(NC)" && exit 1)

test:
	@echo -e ":: $(GREEN)Running tests...$(NC)"
	@go test -cover ./... && echo -e "==> $(BLUE)All tests passed$(NC)" || (echo -e "==> $(RED)Tests failed$(NC)" && exit 1)

.PHONY: test

mocks:
	@echo -e ":: $(GREEN)Generating mocks...$(NC)"
	@mockery && echo -e "==> $(BLUE)Mocks generated successfully$(NC)" || (echo -e "==> $(RED)Mock generation failed$(NC)" && exit 1)

.PHONY: mocks

gen:
	@echo -e ":: $(GREEN)Generating schema and code...$(NC)"
	@echo -e "  -> Running schema creation script..."
	@./scripts/create_sqlc_full_schema.sh || (echo -e "  -> $(RED)Schema creation failed$(NC)" && exit 1)
	@echo -e "  -> Generating SQLC code..."
	@sqlc generate || (echo -e "  -> $(RED)SQLC generation failed$(NC)" && exit 1)
#	@echo -e "  -> Generating Casbin Policyfile..."
#	@./scripts/create_casbin_full_policy.sh || (echo -e "  -> $(RED)Policyfile generation failed$(NC)" && exit 1)
#	@echo -e "  -> Running go generate..."
	@go generate ./... || (echo -e "  -> $(RED)Go generate failed$(NC)" && exit 1)
	@echo -e "==> $(BLUE)Generation completed$(NC)"