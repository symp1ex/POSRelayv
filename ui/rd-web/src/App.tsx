import MainWindow from "./components/MainWindow";
import RDViewer from "./components/RDViewer";
import { getBootstrapConfig } from "./lib/bootstrap";

export default function App() {
  const { isSessionWindow, sessionID } = getBootstrapConfig();

  if (isSessionWindow && sessionID) {
    return <RDViewer sessionID={sessionID} />;
  }

  return <MainWindow />;
}