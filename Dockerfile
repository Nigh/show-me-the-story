# Start from the official lightweight Go Alpine image
FROM golang:1.27-alpine

# Install nodejs and npm using Alpine's package manager (apk)
RUN apk add --no-cache nodejs npm

# 2. Set the working directory inside the container
WORKDIR /app

# 3. Update repositories and install packages using apk
# Using --no-cache avoids saving the package index cache, keeping the image small.
RUN apk add --no-cache curl bash go-task-task
# go-task = 'go-task' | go-task-task = 'task' | stupid but whatever.

# 4. Copy your local project files into the container (optional)
COPY . .

RUN rm ./show-me-the-story

RUN task build

#RUN chmod +x ./show-me-the-story

# 5. Define the default command to execute when the container starts
CMD ["./show-me-the-story", "/app/stories"]

