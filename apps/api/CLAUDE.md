# CLAUDE.md — apps/api (Go 백엔드)

> 루트 `CLAUDE.md`의 제품/도메인 규칙과 함께 적용된다. 이 파일은 `apps/api` 구현 세부사항만 다룬다.

## 앱 소개

Go로 작성된 REST API 서버. 파이프라인 각 단계(조사/생성/메타데이터) 실행과 상태 기록, 외부 API 연동(네이버 데이터랩/LLM/파일 파서), DB 접근을 담당한다.

## 모듈/패키지 구조

```
apps/api
  go.mod                 # 모듈명: blog-pipeline-api
  /cmd/server            # main.go — 서버 진입점
  /internal
    /pipeline            # 상태 머신 로직
    /handler             # HTTP 핸들러
    /store                # pgx 기반 DB 접근(repository)
    /external
      /naverdatalab      # 네이버 데이터랩 API 연동
      /naversearch       # 네이버 검색 API 연동 (상위노출 제목/스니펫 조회)
      /llm               # LLM API 연동
      /fileparser        # PDF/엑셀 파서
  /migrations            # golang-migrate SQL 마이그레이션
  .env.example
```

패키지 분리 기준: `internal/pipeline`(상태 머신) / `internal/handler`(HTTP) / `internal/store`(DB 접근) / `internal/external/*`(외부 API 연동)로 나눈다.

## DB 스키마

PostgreSQL. 스키마 전문은 `apps/api/migrations/0001_init_schema.up.sql`에 정의돼 있다(여기서 중복 기술하지 않음). 테이블: `posts`, `uploaded_files`, `research_results`, `drafts`, `review_actions`, `status_transitions`.

- enum성 컬럼(`content_type`, `status`, `input_method` 등)은 native PostgreSQL ENUM이 아니라 **`TEXT + CHECK` 제약**으로 모델링한다 — 카테고리·상태 값이 아직 미확정인 게 많아, 값 추가/변경이 `ALTER TYPE`보다 훨씬 가벼운 쪽을 택했다.
- 마이그레이션/DB 접근은 ORM 없이 **`golang-migrate` + `pgx`** 조합을 쓴다 — 순수 SQL + 명시적 에러 처리가 루트 CLAUDE.md의 Go 코딩 스타일(에러는 반환값으로 처리, panic 금지)과 맞고, 둘 다 활발히 관리되는 라이브러리다.

## 외부 연동 라이브러리

| 용도 | 라이브러리 | 선정 이유 |
|---|---|---|
| 트렌드/키워드 조사 | 네이버 데이터랩 API | 1차 채택. 구글 트렌드는 공식 API 없음 — 필요 시 `pytrends` 등 비공식 라이브러리 검토(안정성 낮음) |
| 상위노출 제목/스니펫 조회 | 네이버 검색 API(`openapi.naver.com/v1/search/blog.json`) | 드래프팅에서 SEO 제목·어조를 참고하기 위해 상위노출 블로그의 제목+스니펫을 조회. 본문 전체는 가져오지 않음(네이버 블로그 iframe 렌더링이라 스크래핑이 까다롭고, 타 콘텐츠 대량 수집이라 스니펫 대비 리스크가 큼). 데이터랩과 같은 네이버 앱 자격증명(`NAVER_CLIENT_ID/SECRET`) 공유 |
| 초안/분석 생성 | `github.com/anthropics/anthropic-sdk-go` (Anthropic Claude API) | 공식 SDK, 활발히 관리됨. 200k 토큰 컨텍스트라 PDF/엑셀 전문을 통째로 넣기 유리하고, tool use로 구조화된 출력(meta_title/description/image_alts)을 안정적으로 받을 수 있음 |
| PDF 텍스트/표 추출 | `github.com/razvandimescu/gopdf` | MIT, 순수 Go, 외부 의존성 0, 표 추출(rows/columns) 지원 — 청약 공고의 소득기준표·공급일정표 추출에 적합. `unidoc/unipdf`는 기능은 강력하지만 AGPL/상용 이중 라이선스라 배제 |
| 엑셀 파싱 | `github.com/xuri/excelize/v2` | 사실상 Go 생태계 표준, 활발히 관리됨 |

각각 `internal/external/{naverdatalab,naversearch,llm,fileparser}`에서 래핑한다.

## REST API

`internal/handler`가 노출하는 엔드포인트(Go 1.22+ 표준 `net/http` 패턴 매칭 라우팅, 별도 라우터 라이브러리 없음):

| 메서드/경로 | 설명 |
|---|---|
| `GET /posts` | 전체 목록 |
| `GET /posts/{id}` | 단건 조회 (없으면 404) |
| `POST /posts` | 키워드 입력 생성 (`content_type`, `category`, `subtype?`, `keyword` JSON). `status=researching`으로 생성만 하고 끝 — **네이버 데이터랩 조사 실행은 아직 없음**(스텁), 상태가 자동으로 진행되지 않는다 |
| `POST /posts/upload` | PDF/엑셀 업로드(multipart: `file`, `content_type`, `category`, `subtype?`). 저장 → `fileparser`로 텍스트 추출 → 성공 시 `status=researched`, 실패 시 `status=failed_file_parsing` + 에러 메시지. 둘 다 201 응답, 파일은 `UPLOAD_DIR`에 저장 |
| `POST /posts/{id}/draft` | 초안 생성/재생성. `researched`/`needs_revision` 상태에서만 가능(그 외 400). `naversearch`로 상위노출 제목/스니펫 조회(실패해도 무시하고 진행 — 보강 기능이라 단일 장애점 아님) → `llm.GenerateDraft`(Anthropic tool_use로 구조화된 출력) → 성공 시 `drafts`에 새 버전 저장 후 `draft_ready`→`pending_review`까지 자동 연쇄 전이. LLM 호출 실패는 치명적(`failed_drafting` + 에러 메시지, 502) |

`pending_review` 도달 후 승인/반려/보관 엔드포인트는 아직 없다 — 사람이 검토해서 액션을 트리거하는 UI/API가 다음 단계.

## 테스트

- 일반 유닛 테스트: `go test ./...`
- DB에 실제로 붙는 통합 테스트는 `//go:build integration` 태그로 분리해 기본 실행에서 제외한다(`internal/store/store_integration_test.go` 등). `github.com/fergusstrange/embedded-postgres`로 실제 Postgres 바이너리를 사용자 권한으로 띄워 검증한다 — Docker 접근 권한이 없는 환경(샌드박스 등)에서도 root/`docker` 그룹 없이 실행 가능해서 채택했다.
- 실행: `go test -tags=integration ./...`

## 로컬 개발

```bash
# 1. DB 기동 (루트에서)
docker compose up -d db

# 2. 마이그레이션 적용 (golang-migrate CLI 필요: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest)
migrate -path apps/api/migrations -database "$DATABASE_URL" up

# 3. API 서버 실행
go run ./cmd/server
```

`.env.example`을 복사해 `.env`를 만들고 값을 채운다(`DATABASE_URL`, `UPLOAD_DIR`, `NAVER_CLIENT_ID/SECRET`, `ANTHROPIC_API_KEY`).
