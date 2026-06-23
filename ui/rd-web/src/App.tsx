import RDViewer from "./components/RDViewer";
import { getBootstrapConfig } from "./lib/bootstrap";

export default function App() {
  const { sessionID } = getBootstrapConfig();
  return <RDViewer sessionID={sessionID} />;
}
