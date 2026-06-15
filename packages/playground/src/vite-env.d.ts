/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_GATEWAY_ENDPOINT?: string;
  readonly VITE_GATEWAY_TOKEN?: string;
  readonly VITE_DEFAULT_BUCKET?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
