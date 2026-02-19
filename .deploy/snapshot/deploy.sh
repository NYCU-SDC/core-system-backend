# echo "$(date '+%Y-%m-%d %H:%M:%S') [INFO] Deploying Start" >> ./deploy.log

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

export VERSION="pr-$PR_NUMBER"
SEED_FILE="$(cd "$(dirname "$0")" && pwd)/seed.sql"

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

if [ -f "$SEED_FILE" ]; then
    cat "$SEED_FILE" | docker compose exec -T postgres psql -U postgres -d core_system -v ON_ERROR_STOP=1
fi

