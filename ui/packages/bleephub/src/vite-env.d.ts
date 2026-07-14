/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_BLEEPHUB_VERSION?: string;
  readonly VITE_BLEEPHUB_PUBLISHED_AT?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
