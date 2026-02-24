set -e

error_handling() {
    cd ~
    if [ -d "$VERSION" ]; then
        cd "$VERSION"
        docker logs "$VERSION"
        docker compose down
        cd ..
        rm -r "$VERSION"
    fi
    exit 1
}

export VERSION="$COMMIT_SHA"

enable_error_handling="false"
[ ! -d "$VERSION" ] && enable_error_handling="true"

mkdir -p "$VERSION" || true
envsubst < "./compose.yaml" > "./"$VERSION"/compose.yaml" 
cd "$VERSION"

docker compose down
docker compose pull
if [ "$enable_error_handling" == "true" ]; then
    docker compose up -d --wait || error_handling
else
    docker compose up -d --wait
fi