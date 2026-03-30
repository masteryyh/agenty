ARG PG_MAJOR_VERSION=18

FROM postgres:${PG_MAJOR_VERSION}-alpine AS builder-base

RUN apk add --no-cache alpine-sdk curl ca-certificates \
    clang19 clang19-libs clang19-libclang openblas-dev \
    postgresql${PG_MAJOR_VERSION}-dev openssl-libs-static

FROM builder-base AS pgvector-build

ARG PGVECTOR_VERSION=0.8.2

WORKDIR /app

RUN git clone --branch v${PGVECTOR_VERSION} https://github.com/pgvector/pgvector.git && \
    cd pgvector && \
    make clean && make OPTFLAGS="" USE_PGXS=1 -j$(nproc)

FROM builder-base AS pg-search-build

ARG PG_MAJOR_VERSION
ARG PG_SEARCH_VERSION=0.22.1

WORKDIR /app

RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- --default-toolchain stable -y

ENV PATH="/root/.cargo/bin:${PATH}"

RUN git clone --branch v${PG_SEARCH_VERSION} https://github.com/paradedb/paradedb.git && \
    cd paradedb && \
    PGRX_VERSION=$(grep 'pgrx =' Cargo.toml | head -n 1 | cut -d '"' -f 2 | tr -d '=') && \
    cargo install --locked cargo-pgrx --version "${PGRX_VERSION}" && \
    cargo pgrx init "--pg${PG_MAJOR_VERSION}=/usr/local/bin/pg_config" && \
    RUSTFLAGS="-C target-feature=-crt-static" cargo pgrx package --profile=release-with-debug --pg-config "/usr/local/bin/pg_config" --package pg_search && \
    LIB_PATH="/app/paradedb/target/release-with-debug/pg_search-pg${PG_MAJOR_VERSION}/usr/local/lib/postgresql/pg_search.so" && \
    objcopy --only-keep-debug "${LIB_PATH}" "${LIB_PATH}.dbg" && \
    objcopy --strip-debug "${LIB_PATH}" && \
    objcopy --add-gnu-debuglink="${LIB_PATH}.dbg" "${LIB_PATH}"

FROM postgres:${PG_MAJOR_VERSION}-alpine AS final

ARG PG_MAJOR_VERSION

LABEL author="masteryyh"
LABEL description="PostgreSQL image with pgvector and pg_search extensions pre-installed"

COPY --from=pgvector-build /app/pgvector/*.so /usr/local/lib/postgresql/
COPY --from=pgvector-build /app/pgvector/*.control /usr/local/share/postgresql/extension/
COPY --from=pgvector-build /app/pgvector/sql/*.sql /usr/local/share/postgresql/extension/

COPY --from=pg-search-build /app/paradedb/target/release-with-debug/pg_search-pg${PG_MAJOR_VERSION}/usr/local/lib/postgresql/*.so /usr/local/lib/postgresql/
COPY --from=pg-search-build /app/paradedb/target/release-with-debug/pg_search-pg${PG_MAJOR_VERSION}/usr/local/share/postgresql/extension/*.control /usr/local/share/postgresql/extension/
COPY --from=pg-search-build /app/paradedb/target/release-with-debug/pg_search-pg${PG_MAJOR_VERSION}/usr/local/share/postgresql/extension/*.sql /usr/local/share/postgresql/extension/
