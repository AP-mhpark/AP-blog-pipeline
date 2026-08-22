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
| 상위노출 제목/스니펫 조회 | 네이버 검색 API(`openapi.naver.com/v1/search/blog.json`) | 드래프팅에서 SEO 제목·어조를 참고하기 위해 상위노출 블로그의 제목+스니펫을 조회. 본문 전체는 가져오지 않음(네이버 블로그 iframe 렌더링이라 스크래핑이 까다롭고, 타 콘텐츠 대량 수집이라 스니펫 대비 리스크가 큼). 데이터랩과 같은 네이버 앱 자격증명(`NAVER_CLIENT_ID/SECRET`) 공유 — 단, 아래 "네이버 API 발급 제약" 참고 |
| 초안/분석 생성 | `github.com/anthropics/anthropic-sdk-go` (Anthropic Claude API) | 공식 SDK, 활발히 관리됨. 200k 토큰 컨텍스트라 PDF/엑셀 전문을 통째로 넣기 유리하고, tool use로 구조화된 출력(meta_title/description/used_images)을 안정적으로 받을 수 있음 |
| PDF 텍스트/표 추출 | `github.com/razvandimescu/gopdf` | MIT, 순수 Go, 외부 의존성 0, 표 추출(rows/columns) 지원 — 청약 공고의 소득기준표·공급일정표 추출에 적합. `unidoc/unipdf`는 기능은 강력하지만 AGPL/상용 이중 라이선스라 배제 |
| PDF 내장 이미지 추출 | `github.com/pdfcpu/pdfcpu` | 순수 Go, 활발히 관리됨. `api.ExtractImagesFile`로 PDF에 실제 임베드된 이미지(사진·지도 등)를 뽑아 드래프팅에 활용. 표는 대개 이미지가 아니라 PDF 네이티브 텍스트+선이라 이걸로 안 뽑힘 — 표는 대신 LLM이 마크다운 표로 작성 |
| 엑셀 파싱 | `github.com/xuri/excelize/v2` | 사실상 Go 생태계 표준, 활발히 관리됨 |

각각 `internal/external/{naverdatalab,naversearch,llm,fileparser}`에서 래핑한다.

### 네이버 API 발급 제약 (2026-08 확인)

developers.naver.com "API 제휴 신청" 페이지 기준:

- **네이버 검색 API(`naversearch`)는 신규 발급이 막혀 있다.** "검색 API와 한글인명로마자 변환 신규 제휴는 받지 않습니다" — 애플리케이션 등록 시 "사용 API" 목록에도 검색이 안 뜬다. 별도 사업 제휴 제안(navercorp.com 제휴제안) 창구밖에 없고 승인도 보장 안 됨. **개인/신규 개발자는 사실상 발급 불가.**
- **네이버 데이터랩 API(`naverdatalab`, 아직 미구현)는 "제휴 신청"으로는 이론상 가능**하지만(일 허용량 1,000회), 반려 사유에 "API를 테스트 또는 학습의 목적으로 사용하는 경우"가 명시돼 있고 "실제 애플리케이션 화면 캡쳐"를 요구한다 — 아직 발행 전인 이 프로젝트 단계에서는 반려 가능성이 높다. **실제로 네이버 블로그에 글을 몇 개 발행해 서비스 화면이 갖춰진 뒤 신청하는 게 현실적.**
- **영향**: `naversearch` 호출 실패는 이미 non-fatal로 설계돼 있어(`internal/handler` `draftPost`) 크리덴셜이 없어도 드래프팅 자체는 정상 동작한다 — 참고 제목/스니펫만 빠진다. `naverdatalab`(키워드 조사)은 애초에 미구현 상태라 키워드 입력 글은 `researching`에 머무는 기존 동작 그대로다.
- **대안**: 아래 "대체 방안" 섹션 참고.

### 대체 방안 (검색/데이터랩 API 발급 전)

- **상위노출 참고(제목/어조)**: 당장은 Claude 자체 지식으로만 생성(이미 그렇게 동작 중, non-fatal). 필요하면 나중에 "사용자가 네이버에서 직접 검색해 상위 제목 몇 개를 붙여넣는" 수동 입력 필드를 드래프팅 요청에 추가하는 방향을 검토 — API 없이도 같은 효과. **검색 결과 페이지(SERP) 스크래핑은 하지 않는다** — ToS 위반 소지가 있고, 본문 스크래핑을 배제한 것과 같은 이유로 배제.
- **트렌드/키워드 조사**: `pytrends`(비공식) 검토는 이미 CLAUDE.md에 명시돼 있음. 다만 file_input(PDF/엑셀) 경로가 이미 우선 검증된 핵심 흐름이라, keyword_input 자동 조사는 급하지 않음 — 사용자가 키워드를 직접 입력하는 현재 방식으로 계속 진행 가능.

## REST API

`internal/handler`가 노출하는 엔드포인트(Go 1.22+ 표준 `net/http` 패턴 매칭 라우팅, 별도 라우터 라이브러리 없음):

| 메서드/경로 | 설명 |
|---|---|
| `GET /posts` | 전체 목록 |
| `GET /posts/{id}` | 단건 조회 (없으면 404) |
| `DELETE /posts/{id}` | 삭제(204). 관련 데이터(업로드 파일 레코드, 조사 결과, 초안, 리뷰 기록, 상태 이력)는 FK `ON DELETE CASCADE`로 함께 삭제. 상태 제한 없음(1인용 내부 툴이라 테스트/실수 데이터를 언제든 지울 수 있어야 함) |
| `POST /posts` | 키워드 입력 생성 (`content_type`, `category`, `subtype?`, `keyword` JSON). `status=researching`으로 생성만 하고 끝 — **네이버 데이터랩 조사 실행은 아직 없음**(스텁), 상태가 자동으로 진행되지 않는다 |
| `POST /posts/upload` | PDF/엑셀 업로드(multipart: `file`, `content_type`, `category`, `subtype?`). 저장 → `fileparser`로 텍스트 추출 → 성공 시 `status=researched`, 실패 시 `status=failed_file_parsing` + 에러 메시지. 둘 다 201 응답, 파일은 `UPLOAD_DIR`에 저장. PDF는 텍스트와 함께 내장 이미지도 추출(`fileparser.ExtractPDFImages`, 비치명적) → `research_results.raw_data`에 `{"images": [...]}`로 기록 |
| `POST /posts/{id}/draft` | 초안 생성/재생성. `researched`/`needs_revision` 상태에서만 가능(그 외 400). file_input 글은 검색 쿼리를 `llm.ExtractKeyword`(Haiku, 원문에서 키워드 한 줄 추출)로 정하고 실패하면 `category+subtype` 폴백(비치명적) — keyword_input 글은 그대로 `input_keyword` 사용. 그 쿼리로 `naversearch` 상위노출 제목/스니펫 조회(실패해도 무시하고 진행 — 보강 기능이라 단일 장애점 아님) → `llm.GenerateDraft`(Anthropic tool_use로 구조화된 출력, 추출된 이미지 파일명 목록도 함께 전달) → 성공 시 `drafts`에 새 버전 저장 후 `draft_ready`→`pending_review`까지 자동 연쇄 전이. LLM 호출(초안 생성) 실패는 치명적(`failed_drafting` + 에러 메시지, 502) |
| `GET /posts/{id}/drafts` | 해당 글의 모든 초안 버전(최신순). `used_images` 필드에 본문에서 실제 참조한 추출 이미지 파일명 목록(DB 컬럼명은 `image_alts` 그대로, 마이그레이션 없이 의미만 재정의) |
| `GET /uploads/{path}` | 업로드 원본 파일 + 추출 이미지 정적 서빙(`UPLOAD_DIR` 그대로 노출). 인증 없는 1인용 툴이고 원문이 공개 공고문이라 문제 없음 |
| `POST /posts/{id}/approve` | `pending_review`→`approved`. 초안이 없으면 422, 상태가 안 맞으면 400. `review_actions`에 `approve` 기록 |
| `POST /posts/{id}/reject` | `pending_review`→`needs_revision` (JSON body `{"feedback_note"?: string}`). `review_actions`에 `reject`+피드백 기록. 재생성은 기존 `POST /posts/{id}/draft` 재사용(`needs_revision`→`drafting` 지원) |
| `POST /posts/{id}/archive` | `approved`→`archived` — 사용자가 네이버 에디터에 수동 업로드 완료했다는 표시. `review_actions` 관여 없음 |

이제 파이프라인 상태 머신 전체(`researching`~`archived`, 반려 루프 포함)가 API로 끝까지 연결됐다.

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
