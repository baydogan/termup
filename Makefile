IMAGE := zerotolerance
CONTAINER := ztd

.PHONY: up down

# Build the image and start a fresh container (detached).
up:
	docker build -t $(IMAGE) .
	docker run -d --name $(CONTAINER) $(IMAGE)

# Remove the old container and image.
down:
	-docker rm -f $(CONTAINER)
	-docker rmi $(IMAGE)
