# CLAUDE.md — apps/web (React 프론트)

> 루트 `CLAUDE.md`의 제품/도메인 규칙과 함께 적용된다. 이 파일은 `apps/web` 구현 세부사항만 다룬다.

## 앱 소개

React(Vite) 기반 검토/승인 UI. 순수 클라이언트 앱이며 SSR/SEO는 불필요하다(Next.js 미사용). 목록 화면(진행 중인 포스트 상태 확인) + 상세 화면(초안 보기/수정/승인·반려 버튼)으로 구성된다.

## 디렉토리 구조

```
apps/web
  /src
    /pages       # 목록/상세 화면
    /components
    /api         # 백엔드 API 클라이언트
  .env.example
```

## 코딩 스타일

- 컴포넌트는 PascalCase, 파일명은 kebab-case를 기본으로 한다.
- 그 외 일반 규칙(주석, 커밋 컨벤션 등)은 루트 CLAUDE.md 참고.

## 로컬 개발

```bash
npm install
npm run dev
```

`.env.example`을 복사해 `.env`를 만들고 `VITE_API_BASE_URL`을 채운다(API 서버는 `apps/api/CLAUDE.md` 참고해 별도 실행).
