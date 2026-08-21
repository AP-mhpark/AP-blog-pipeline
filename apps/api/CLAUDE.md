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
| 초안/분석 생성 | `github.com/anthropics/anthropic-sdk-go` (Anthropic Claude API) | 공식 SDK, 활발히 관리됨. 200k 토큰 컨텍스트라 PDF/엑셀 전문을 통째로 넣기 유리하고, tool use로 구조화된 출력(meta_title/description/image_alts)을 안정적으로 받을 수 있음 |
| PDF 텍스트/표 추출 | `github.com/razvandimescu/gopdf` | MIT, 순수 Go, 외부 의존성 0, 표 추출(rows/columns) 지원 — 청약 공고의 소득기준표·공급일정표 추출에 적합. `unidoc/unipdf`는 기능은 강력하지만 AGPL/상용 이중 라이선스라 배제 |
| 엑셀 파싱 | `github.com/xuri/excelize/v2` | 사실상 Go 생태계 표준, 활발히 관리됨 |

각각 `internal/external/{naverdatalab,llm,fileparser}`에서 래핑한다.

## 로컬 개발

```bash
# 1. DB 기동 (루트에서)
docker compose up -d db

# 2. 마이그레이션 적용 (golang-migrate CLI 필요: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest)
migrate -path apps/api/migrations -database "$DATABASE_URL" up

# 3. API 서버 실행
go run ./cmd/server
```

`.env.example`을 복사해 `.env`를 만들고 값을 채운다(`DATABASE_URL`, `NAVER_DATALAB_CLIENT_ID/SECRET`, `ANTHROPIC_API_KEY`).
