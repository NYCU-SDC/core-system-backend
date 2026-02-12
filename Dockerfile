FROM golang:1.24

WORKDIR /app

COPY bin/backend /app/backend
COPY internal/database/migrations /app/migrations
COPY internal/auth/casbin/model.conf /app/model.conf
COPY internal/auth/casbin/policy.csv /app/policy.csv

ENV CASBIN_MODEL_PATH=/app/model.conf
ENV CASBIN_POLICY_PATH=/app/policy.csv

EXPOSE 8080

CMD ["/app/backend"]
