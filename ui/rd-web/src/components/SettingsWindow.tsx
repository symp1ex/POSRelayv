import { useEffect, useMemo, useState } from "react";
import mainIcon from "../assets/main.png";
import type { JsonSettingObject, JsonSettingValue, SettingsConfigFile } from "../lib/bridge";

function isPlainObject(value: JsonSettingValue | undefined): value is JsonSettingObject {
    return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function isSelectSetting(value: JsonSettingValue | undefined): value is { active: string; list: string[] } {
    if (!isPlainObject(value)) {
        return false;
    }

    return (
        typeof value.active === "string" &&
        Array.isArray(value.list) &&
        value.list.every((item) => typeof item === "string")
    );
}

function cloneConfigs(configs: SettingsConfigFile[]) {
    return JSON.parse(JSON.stringify(configs)) as SettingsConfigFile[];
}

function settingTitle(value: string) {
    return value.replaceAll("_", " ");
}

export default function SettingsWindow() {
    const [configs, setConfigs] = useState<SettingsConfigFile[]>([]);
    const [activeConfigName, setActiveConfigName] = useState("");
    const [statusText, setStatusText] = useState("Loading settings...");
    const [isLoading, setIsLoading] = useState(true);
    const [isSaving, setIsSaving] = useState(false);

    const activeConfig = useMemo(
        () => configs.find((config) => config.name === activeConfigName) ?? configs[0] ?? null,
        [configs, activeConfigName],
    );

    useEffect(() => {
        function preventBrowserZoomByWheel(event: WheelEvent) {
            if (event.ctrlKey) {
                event.preventDefault();
            }
        }

        function preventBrowserZoomByKeyboard(event: KeyboardEvent) {
            if (!event.ctrlKey) {
                return;
            }

            if (
                event.key === "+" ||
                event.key === "-" ||
                event.key === "=" ||
                event.key === "0" ||
                event.code === "NumpadAdd" ||
                event.code === "NumpadSubtract" ||
                event.code === "Digit0"
            ) {
                event.preventDefault();
            }
        }

        window.addEventListener("wheel", preventBrowserZoomByWheel, {
            passive: false,
            capture: true,
        });

        window.addEventListener("keydown", preventBrowserZoomByKeyboard, {
            capture: true,
        });

        return () => {
            window.removeEventListener("wheel", preventBrowserZoomByWheel, {
                capture: true,
            });

            window.removeEventListener("keydown", preventBrowserZoomByKeyboard, {
                capture: true,
            });
        };
    }, []);

    useEffect(() => {
        void loadConfigs();
    }, []);

    function dragWindow() {
        window.settingsWindowDrag?.();
    }

    function minimizeWindow() {
        window.settingsWindowMinimize?.();
    }

    function closeWindow() {
        window.settingsWindowClose?.();
    }

    async function loadConfigs() {
        if (!window.loadSettingsConfigs) {
            setStatusText("Bridge loadSettingsConfigs недоступен");
            setIsLoading(false);
            return;
        }

        setIsLoading(true);

        try {
            const result = await window.loadSettingsConfigs();

            if (!result.ok) {
                setStatusText(result.message || "Failed to load settings");
                setConfigs([]);
                return;
            }

            const nextConfigs = result.configs ?? [];

            setConfigs(nextConfigs);
            setActiveConfigName(nextConfigs[0]?.name ?? "");
            setStatusText(nextConfigs.length > 0 ? "Settings loaded" : "JSON configs not found");
        } catch (error) {
            const message = error instanceof Error ? error.message : String(error);
            setStatusText(`Error loading settings: ${message}`);
        } finally {
            setIsLoading(false);
        }
    }

    function updateActiveConfig(updater: (draft: JsonSettingObject) => void) {
        setConfigs((current) => {
            const next = cloneConfigs(current);
            const target = next.find((config) => config.name === activeConfig?.name);

            if (target) {
                updater(target.data);
            }

            return next;
        });
    }

    function getSettingByPath(root: JsonSettingObject, path: string[]): JsonSettingValue | undefined {
        let current: JsonSettingValue = root;

        for (const key of path) {
            if (!isPlainObject(current)) {
                return undefined;
            }

            current = current[key];
        }

        return current;
    }

    function setSettingByPath(root: JsonSettingObject, path: string[], value: string | number | boolean) {
        let current: JsonSettingValue = root;

        for (let index = 0; index < path.length - 1; index += 1) {
            const key = path[index];

            if (!isPlainObject(current)) {
                return;
            }

            current = current[key];
        }

        if (!isPlainObject(current)) {
            return;
        }

        current[path[path.length - 1]] = value;
    }

    function updateSelectSetting(path: string[], value: string) {
        updateActiveConfig((draft) => {
            const setting = getSettingByPath(draft, path);

            if (isSelectSetting(setting)) {
                setting.active = value;
            }
        });
    }

    function updatePrimitiveSetting(path: string[], value: string | number | boolean) {
        updateActiveConfig((draft) => {
            setSettingByPath(draft, path, value);
        });
    }

    async function saveActiveConfig() {
        if (!activeConfig) {
            setStatusText("There is no active config to save");
            return;
        }

        if (!window.saveSettingsConfig) {
            setStatusText("Bridge saveSettingsConfig недоступен");
            return;
        }

        setIsSaving(true);

        try {
            const result = await window.saveSettingsConfig(activeConfig.name, activeConfig.data);

            if (!result.ok) {
                setStatusText(result.message || "Failed to save settings");
                return;
            }

            setStatusText(result.message || "Settings saved");
        } catch (error) {
            const message = error instanceof Error ? error.message : String(error);
            setStatusText(`Saving error: ${message}`);
        } finally {
            setIsSaving(false);
        }
    }

    function renderPrimitiveSetting(path: string[], label: string, value: JsonSettingValue) {
        const key = path.join(".");

        if (typeof value === "boolean") {
            return (
                <label key={key} className="settings-checkbox-row">
                    <input
                        type="checkbox"
                        checked={value}
                        onChange={(event) => updatePrimitiveSetting(path, event.target.checked)}
                    />
                    <span>{settingTitle(label)}</span>
                </label>
            );
        }

        if (typeof value === "number") {
            return (
                <label key={key} className="settings-field">
                    <span>{settingTitle(label)}</span>
                    <input
                        type="number"
                        value={value}
                        onChange={(event) => {
                            const parsed = Number(event.target.value);
                            updatePrimitiveSetting(path, Number.isNaN(parsed) ? 0 : parsed);
                        }}
                    />
                </label>
            );
        }

        if (typeof value === "string") {
            return (
                <label key={key} className="settings-field">
                    <span>{settingTitle(label)}</span>
                    <input
                        type="text"
                        value={value}
                        onChange={(event) => updatePrimitiveSetting(path, event.target.value)}
                    />
                </label>
            );
        }

        return null;
    }

    function renderSelectSetting(path: string[], label: string, value: { active: string; list: string[] }) {
        return (
            <label key={path.join(".")} className="settings-field">
                <span>{settingTitle(label)}</span>
                <select
                    value={value.active}
                    onChange={(event) => updateSelectSetting(path, event.target.value)}
                >
                    {value.list.map((option) => (
                        <option key={option} value={option}>
                            {option}
                        </option>
                    ))}
                </select>
            </label>
        );
    }

    function renderSetting(path: string[], label: string, value: JsonSettingValue, depth = 0) {
        if (isSelectSetting(value)) {
            return renderSelectSetting(path, label, value);
        }

        if (isPlainObject(value)) {
            return (
                <div
                    key={path.join(".")}
                    className={depth > 0 ? "settings-nested-group" : "settings-block__fields"}
                >
                    {depth > 0 ? (
                        <div className="settings-nested-title">
                            {settingTitle(label)}
                        </div>
                    ) : null}

                    <div className="settings-block__fields">
                        {Object.entries(value)
                            .filter(([fieldKey]) => fieldKey !== "active" && fieldKey !== "list")
                            .map(([fieldKey, fieldValue]) =>
                                renderSetting([...path, fieldKey], fieldKey, fieldValue, depth + 1),
                            )}
                    </div>
                </div>
            );
        }

        return renderPrimitiveSetting(path, label, value);
    }

    function renderBlock(blockKey: string, blockValue: JsonSettingValue) {
        return (
            <section key={blockKey} className="settings-block">
                <h2>{settingTitle(blockKey)}</h2>

                {isSelectSetting(blockValue) ? (
                    renderSelectSetting([blockKey], blockKey, blockValue)
                ) : isPlainObject(blockValue) ? (
                    <div className="settings-block__fields">
                        {Object.entries(blockValue)
                            .filter(([fieldKey]) => fieldKey !== "active" && fieldKey !== "list")
                            .map(([fieldKey, value]) =>
                                renderSetting([blockKey, fieldKey], fieldKey, value),
                            )}
                    </div>
                ) : (
                    renderPrimitiveSetting([blockKey], blockKey, blockValue)
                )}
            </section>
        );
    }

    return (
        <main className="settings-window" aria-label="Settings POSRelayv">
            <header
                className="settings-titlebar"
                aria-label="Window panel"
                onPointerDown={(event) => {
                    if (event.button === 0) {
                        dragWindow();
                    }
                }}
            >
                <div className="settings-titlebar__brand">
                    <span className="settings-titlebar__logo">
                        <img src={mainIcon} alt="" className="main-icon ph-icon--title" />
                    </span>
                    <span>POSRelayv</span>
                </div>

                <div className="settings-titlebar__actions">
                    <button
                        type="button"
                        className="settings-titlebar__button"
                        aria-label="Collapse"
                        onPointerDown={(event) => event.stopPropagation()}
                        onClick={minimizeWindow}
                    >
                        −
                    </button>

                    <button
                        type="button"
                        className="settings-titlebar__button settings-titlebar__button--close"
                        aria-label="Closed"
                        onPointerDown={(event) => event.stopPropagation()}
                        onClick={closeWindow}
                    >
                        ×
                    </button>
                </div>
            </header>

            <section className="settings-layout">
                <aside className="settings-sidebar">
                    <div className="settings-sidebar__title">Settings</div>

                    {configs.map((config) => (
                        <button
                            key={config.name}
                            type="button"
                            className={activeConfig?.name === config.name ? "settings-nav-item settings-nav-item--active" : "settings-nav-item"}
                            onClick={() => {
                                setActiveConfigName(config.name);
                                setStatusText(`Load config: ${config.name}`);
                            }}
                        >
                            {config.name}
                        </button>
                    ))}
                </aside>

                <section className="settings-content-area">
                    <div className="settings-page-header">
                        <div>
                            <h1>{activeConfig?.name ?? "Settings"}</h1>
                            <p>{statusText}</p>
                        </div>
                    </div>

                    {isLoading ? (
                        <div className="settings-empty">Загрузка настроек...</div>
                    ) : configs.length === 0 ? (
                        <div className="settings-empty">В папке configs нет JSON-конфигов</div>
                    ) : (
                        <div className="settings-content">
                            {activeConfig ? Object.entries(activeConfig.data).map(([blockKey, blockValue]) => renderBlock(blockKey, blockValue)) : null}
                        </div>
                    )}

                    <footer className="settings-footer">
                        <button
                            type="button"
                            className="settings-secondary-button"
                            onClick={() => void loadConfigs()}
                            disabled={isLoading || isSaving}
                        >
                            Restore
                        </button>

                        <button
                            type="button"
                            className="settings-save-button"
                            onClick={() => void saveActiveConfig()}
                            disabled={isLoading || isSaving || !activeConfig}
                        >
                            {isSaving ? "Saving..." : "Save"}
                        </button>
                    </footer>
                </section>
            </section>
        </main>
    );
}