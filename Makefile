.PHONY: setup run snapshot

# Ensure seed files exist so the Docker build can bake them in.
# If starting fresh, creates an empty apps list; add apps via the UI then run `make snapshot`.
setup:
	@mkdir -p ./data/reviews
	@test -f ./data/apps.json || echo '[]' > ./data/apps.json
	@echo "data ready"

# Extract the latest reviews from the running container so they survive a rebuild.
snapshot:
	docker cp runway-backend:/data/apps.json ./data/apps.json
	docker cp runway-backend:/data/reviews ./data/
	@echo "snapshot done — run 'make run' to rebuild with updated seed data"

run:
	docker compose up --build
