import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import SettingsWindow from "./components/SettingsWindow";
import "./index.css";
import "./styles/settings-window.css";

createRoot(document.getElementById("root")!).render(
    <StrictMode>
        <SettingsWindow />
    </StrictMode>,
);