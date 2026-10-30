/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Base URL of the TracePilot backend, for a console hosted apart from it. */
  readonly VITE_API_BASE_URL?: string;
}
