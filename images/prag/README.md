# PRAG - Custom built PostgreSQL image for RAG scenarios

This image is based on the official PostgreSQL Alpine image, with the pgvector and pg_search extensions pre-installed and configured.

## Quick Start

Build image (if you want to):

```bash
docker build -t masteryyh/prag:18-alpine .
```

Run container:

```bash
docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=sOmEpAsSwOrD --name agenty-postgres masteryyh/prag:18-alpine
```

Enable pgvector extension:

```bash
docker exec -it agenty-postgres psql -U postgres -c "CREATE EXTENSION IF NOT EXISTS vector;"
```

Enable pg_search extension:

```bash
docker exec -it agenty-postgres psql -U postgres -c "CREATE EXTENSION IF NOT EXISTS pg_search;"
```

## License

Copyright © 2026 masteryyh

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
