import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app/app";
import "./styles/fonts.css";
import "./styles/base.css";
import "./styles/components.css";
import "./styles/glass.css";
import "./styles/notice.css";
import "./styles/responsive.css";

createRoot(document.getElementById("root")!).render(<StrictMode><App /></StrictMode>);
