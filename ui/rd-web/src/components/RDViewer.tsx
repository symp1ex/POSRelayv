import { useEffect } from "react";
import { useRDSession } from "../hooks/useRDSession";

type RDViewerProps = {
    sessionID: string;
    stretch: boolean;
};

export default function RDViewer({ sessionID, stretch }: RDViewerProps) {
    const { statusText, videoRef } = useRDSession(sessionID);

    useEffect(() => {
        window.rdWindowReady?.();
    }, []);

    return (
        <div className={stretch ? "rd-root rd-root--stretch" : "rd-root"}>
            <div className="rd-stage">
                <video
                    ref={videoRef}
                    id="rd-video"
                    className="rd-video"
                    autoPlay
                    playsInline
                    muted
                    preload="auto"
                    draggable={false}
                />
            </div>

            <div className="rd-status" id="status">
                {statusText}
            </div>
        </div>
    );
}
