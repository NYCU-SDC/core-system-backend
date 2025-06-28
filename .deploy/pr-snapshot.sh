# echo "$(date '+%Y-%m-%d %H:%M:%S') [INFO] Deploying Start" >> ./deploy.log

set -e

deploy() {
    local dir=$1
    local use_envsubst=$2
    local enable_error_handling=$3

    mkdir -p "$dir" || true

    envsubst < "./compose.yaml" > "./$dir/compose.yaml"

    cd "$dir"

    docker compose down
    docker compose pull
    if [ "$enable_error_handling" == "true" ]; then
        docker compose up -d --wait || error_handling
    else
        docker compose up -d --wait
    fi

    cd ..
}

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

notify() {
    curl -s --header "Content-type: application/json" \
         --request POST \
         --data "$1" \
         "$notificationUrl"
}

build_pr_payload() {
    local mode=$1
    local pr_url="https://github.com/commonground-project/backend/pull/$PR_NUMBER"
    echo "{\"mode\":\"$mode\",\"prUrl\":\"$pr_url\",\"port\":$PORT,\"version\":\"$VERSION\",\"title\":\"$TITLE\",\"isDraft\":false}"
}
if [ "$MODE" == "Snapshot" ] || [ "$MODE" == "Close" ]; then
    export PORT=$((4000 + $PR_NUMBER))
    export VERSION="pr-$PR_NUMBER"
fi

case "$MODE" in
    "Snapshot")
        flag="false"
        [ ! -d "$VERSION" ] && flag="true"

        deploy "$VERSION" "true" "$flag"

#        if [ "$flag" == "true" ]; then
#            notify "$(build_pr_payload prOpen)"
#        fi
        ;;

    "Close")
        cd "$VERSION"
        docker compose down
        cd ..
        rm -r "$VERSION"

#        notify "$(build_pr_payload prClose)"
        ;;

    "Dev")
        export PORT=8082
        export VERSION="dev"

        deploy "$VERSION" "true" "false"

#        notify "{\"mode\":\"dev\"}"
        ;;

    "Stage")
        export VERSION="stage"
        export PORT=8083

        deploy "stage" "false" "false"

#        notify "{\"mode\":\"stage\"}"
        ;;
esac

echo "finish"