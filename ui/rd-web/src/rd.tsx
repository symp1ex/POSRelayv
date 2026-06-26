import ReactDOM from "react-dom/client";
import RDViewer from "./components/RDViewer";
import { getBootstrapConfig } from "./lib/bootstrap";
import "./index.css";
import "./styles/rd.css";

const { sessionID } = getBootstrapConfig();

if (!sessionID) {
    throw new Error("RD session_id is missing");
}

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
    <RDViewer sessionID={sessionID} />,
);