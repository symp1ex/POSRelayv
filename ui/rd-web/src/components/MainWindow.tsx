import { useEffect, useState } from "react";

type RecentConnection = {
    id: string;
    name: string;
    address: string;
    system: string;
    time: string;
    status: "online" | "away" | "offline";
    device: "desktop" | "laptop" | "server";
};

type ActiveTab = "recent" | "contacts";

const recentConnections: RecentConnection[] = [
    {
        id: "office-pc-01",
        name: "Офис — ПК 01",
        address: "192.168.1.10",
        system: "Windows 11 Pro",
        time: "Сегодня, 10:24",
        status: "online",
        device: "desktop",
    },
    {
        id: "ivan-laptop",
        name: "Ноутбук Ивана",
        address: "192.168.1.25",
        system: "Windows 11 Home",
        time: "Вчера, 16:45",
        status: "online",
        device: "laptop",
    },
    {
        id: "accounting-server",
        name: "Сервер бухгалтерии",
        address: "192.168.1.50",
        system: "Windows Server 2019",
        time: "Вчера, 11:32",
        status: "away",
        device: "server",
    },
    {
        id: "design-studio",
        name: "Дизайн-студия",
        address: "192.168.1.77",
        system: "Windows 10 Pro",
        time: "12.05.2024, 09:18",
        status: "online",
        device: "desktop",
    },
    {
        id: "home-pc",
        name: "Домашний ПК",
        address: "192.168.1.88",
        system: "Windows 11 Home",
        time: "10.05.2024, 21:07",
        status: "offline",
        device: "laptop",
    },
];

const contacts: RecentConnection[] = [
    {
        id: "support-pc",
        name: "Поддержка",
        address: "support.local",
        system: "Группа контактов",
        time: "Избранное",
        status: "online",
        device: "desktop",
    },
    {
        id: "admin-server",
        name: "Администратор",
        address: "admin.local",
        system: "Технический контакт",
        time: "Недавно",
        status: "away",
        device: "server",
    },
];

function getDeviceIcon(device: RecentConnection["device"]) {
    if (device === "server") {
        return "▣";
    }

    if (device === "laptop") {
        return "▱";
    }

    return "▢";
}

export default function MainWindow() {
    const [activeTab, setActiveTab] = useState<ActiveTab>("recent");
    const [clientId, setClientId] = useState("");
    const [password, setPassword] = useState("");
    const [isPasswordPopupOpen, setIsPasswordPopupOpen] = useState(false);
    const [isConnecting, setIsConnecting] = useState(false);
    const [selectedServer, setSelectedServer] = useState("Основной сервер");
    const [isServerMenuOpen, setIsServerMenuOpen] = useState(false);
    const [selectedItemId, setSelectedItemId] = useState<string | null>(recentConnections[0]?.id ?? null);
    const [favoriteIds, setFavoriteIds] = useState<Set<string>>(new Set());
    const [actionText, setActionText] = useState("Интерфейс готов");

    const visibleConnections = activeTab === "recent" ? recentConnections : contacts;

    useEffect(() => {
        function preventBrowserZoomByWheel(event: WheelEvent) {
            if (!event.ctrlKey) {
                return;
            }

            event.preventDefault();
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

    function animateAction(text: string) {
        setActionText(text);
    }

    function minimizeWindow() {
        if (window.mainWindowMinimize) {
            window.mainWindowMinimize();
            return;
        }

        animateAction("Сворачивание окна недоступно");
    }

    function closeWindow() {
        if (window.mainWindowClose) {
            window.mainWindowClose();
            return;
        }

        animateAction("Закрытие окна недоступно");
    }

    function dragWindow() {
        if (window.mainWindowDrag) {
            window.mainWindowDrag();
            return;
        }

        animateAction("Перемещение окна недоступно");
    }

    function toggleFavorite(id: string) {
        setFavoriteIds((current) => {
            const next = new Set(current);

            if (next.has(id)) {
                next.delete(id);
            } else {
                next.add(id);
            }

            return next;
        });

        animateAction("Избранное обновлено");
    }

    function selectServer(serverName: string) {
        setSelectedServer(serverName);
        setIsServerMenuOpen(false);
        animateAction(`Выбран сервер: ${serverName}`);
    }

    function openPasswordPopup() {
        const trimmedClientId = clientId.trim();

        if (!trimmedClientId) {
            animateAction("Введите ID клиента");
            return;
        }

        setPassword("");
        setIsPasswordPopupOpen(true);
        animateAction(`Введите пароль для ${trimmedClientId}`);
    }

    async function confirmPassword() {
        const trimmedClientId = clientId.trim();

        if (!trimmedClientId) {
            animateAction("Введите ID клиента");
            return;
        }

        if (!password) {
            animateAction("Введите пароль");
            return;
        }

        if (!window.startHiddenConsole) {
            animateAction("Bridge startHiddenConsole недоступен");
            return;
        }

        setIsConnecting(true);

        try {
            const result = await window.startHiddenConsole(trimmedClientId, password);

            if (!result.ok) {
                animateAction(result.message || "Не удалось запустить подключение");
                return;
            }

            setIsPasswordPopupOpen(false);
            setPassword("");
            animateAction("Консоль запущена скрыто");
        } catch (error) {
            const message = error instanceof Error ? error.message : String(error);
            animateAction(`Ошибка запуска: ${message}`);
        } finally {
            setIsConnecting(false);
        }
    }

    return (
        <main className="main-window" aria-label="Главное окно POSRelay RD">
            <section className="app-shell">
                <header
                    className="custom-titlebar"
                    aria-label="Панель окна"
                    onPointerDown={(event) => {
                        if (event.button !== 0) {
                            return;
                        }

                        dragWindow();
                    }}
                >
                    <div className="custom-titlebar__brand">
                        <span className="custom-titlebar__logo">▣</span>
                        <span>POSRelay RD</span>
                    </div>

                    <div className="custom-titlebar__actions">
                        <button
                            type="button"
                            className="custom-titlebar__button"
                            aria-label="Свернуть"
                            onPointerDown={(event) => event.stopPropagation()}
                            onClick={minimizeWindow}
                        >
                            −
                        </button>

                        <button
                            type="button"
                            className="custom-titlebar__button custom-titlebar__button--close"
                            aria-label="Закрыть"
                            onPointerDown={(event) => event.stopPropagation()}
                            onClick={closeWindow}
                        >
                            ×
                        </button>
                    </div>
                </header>

                <aside className="side-nav" aria-label="Основная навигация">
                    <button
                        type="button"
                        className="side-nav__item side-nav__item--active"
                        aria-label="Подключения"
                        onClick={() => animateAction("Раздел подключений")}
                    >
                        <span className="side-nav__marker" />
                        <span className="side-nav__icon">▣</span>
                    </button>
                </aside>
                <section className="window-card">
                    <section className="top-grid">
                        <div className="panel connection-panel">
                            <label className="field-label" htmlFor="client-id">
                                ID клиента
                            </label>

                            <div className="input-row">
                                <span className="input-icon">♙</span>
                                <input
                                    id="client-id"
                                    value={clientId}
                                    onChange={(event) => setClientId(event.target.value)}
                                    placeholder="Введите ID клиента"
                                />
                            </div>

                            <button
                                type="button"
                                className="primary-button"
                                onClick={openPasswordPopup}
                            >
                                Подключиться
                            </button>
                        </div>

                        <div className="panel server-panel">
                            <span className="field-label">Сервер</span>

                            <div className="server-select">
                                <button
                                    type="button"
                                    className="server-select__button"
                                    aria-expanded={isServerMenuOpen}
                                    onClick={() => setIsServerMenuOpen((value) => !value)}
                                >
                                    <span className="server-icon">▤</span>
                                    <span>{selectedServer}</span>
                                    <span className={`chevron ${isServerMenuOpen ? "chevron--open" : ""}`}>⌄</span>
                                </button>

                                {isServerMenuOpen ? (
                                    <div className="server-menu">
                                        <button type="button" onClick={() => selectServer("Основной сервер")}>
                                            Основной сервер
                                        </button>
                                        <button type="button" onClick={() => selectServer("Резервный сервер")}>
                                            Резервный сервер
                                        </button>
                                        <button type="button" onClick={() => selectServer("Локальный сервер")}>
                                            Локальный сервер
                                        </button>
                                    </div>
                                ) : null}
                            </div>
                        </div>
                    </section>

                    <section className="content-panel">
                        <div className="tabs" role="tablist" aria-label="Списки подключений">
                            <button
                                type="button"
                                role="tab"
                                aria-selected={activeTab === "recent"}
                                className={activeTab === "recent" ? "tab tab--active" : "tab"}
                                onClick={() => {
                                    setActiveTab("recent");
                                    setSelectedItemId(recentConnections[0]?.id ?? null);
                                    animateAction("Последние подключения");
                                }}
                            >
                                Последние подключения
                            </button>

                            <button
                                type="button"
                                role="tab"
                                aria-selected={activeTab === "contacts"}
                                className={activeTab === "contacts" ? "tab tab--active" : "tab"}
                                onClick={() => {
                                    setActiveTab("contacts");
                                    setSelectedItemId(contacts[0]?.id ?? null);
                                    animateAction("Контакты");
                                }}
                            >
                                Контакты
                            </button>
                        </div>

                        <div className="connection-list">
                            {visibleConnections.map((connection) => (
                                <article
                                    key={connection.id}
                                    className={selectedItemId === connection.id ? "connection-item connection-item--active" : "connection-item"}
                                    onClick={() => {
                                        setSelectedItemId(connection.id);
                                        animateAction(`Выбрано: ${connection.name}`);
                                    }}
                                >
                                    <button
                                        type="button"
                                        className="device-button"
                                        aria-label={`Выбрать ${connection.name}`}
                                        onClick={(event) => {
                                            event.stopPropagation();
                                            setSelectedItemId(connection.id);
                                            animateAction(`Открыть карточку: ${connection.name}`);
                                        }}
                                    >
                                        {getDeviceIcon(connection.device)}
                                    </button>

                                    <span className={`status-dot status-dot--${connection.status}`} />

                                    <div className="connection-info">
                                        <h3>{connection.name}</h3>
                                        <p>
                                            {connection.address}
                                            <span>•</span>
                                            {connection.system}
                                        </p>
                                    </div>

                                    <time>{connection.time}</time>

                                    <button
                                        type="button"
                                        className={favoriteIds.has(connection.id) ? "icon-button icon-button--active" : "icon-button"}
                                        aria-label="Добавить в избранное"
                                        onClick={(event) => {
                                            event.stopPropagation();
                                            toggleFavorite(connection.id);
                                        }}
                                    >
                                        ☆
                                    </button>

                                    <button
                                        type="button"
                                        className="icon-button"
                                        aria-label="Дополнительные действия"
                                        onClick={(event) => {
                                            event.stopPropagation();
                                            animateAction(`Меню: ${connection.name}`);
                                        }}
                                    >
                                        ⋮
                                    </button>
                                </article>
                            ))}
                        </div>

                        <div className="action-toast" aria-live="polite">
                            {actionText}
                        </div>
                    </section>
                </section>
            </section>
            {isPasswordPopupOpen ? (
                <div className="modal-backdrop" role="presentation">
                    <section className="password-modal" role="dialog" aria-modal="true" aria-labelledby="password-modal-title">
                        <h2 id="password-modal-title">Введите пароль</h2>

                        <p>
                            Подключение к клиенту <strong>{clientId.trim()}</strong>
                        </p>

                        <div className="password-field">
                            <label htmlFor="client-password">Пароль</label>
                            <input
                                id="client-password"
                                type="password"
                                value={password}
                                autoFocus
                                placeholder="Введите пароль"
                                onChange={(event) => setPassword(event.target.value)}
                                onKeyDown={(event) => {
                                    if (event.key === "Enter") {
                                        void confirmPassword();
                                    }

                                    if (event.key === "Escape") {
                                        setIsPasswordPopupOpen(false);
                                    }
                                }}
                            />
                        </div>

                        <div className="modal-actions">
                            <button
                                type="button"
                                className="secondary-button"
                                disabled={isConnecting}
                                onClick={() => {
                                    setIsPasswordPopupOpen(false);
                                    setPassword("");
                                    animateAction("Подключение отменено");
                                }}
                            >
                                Отмена
                            </button>

                            <button
                                type="button"
                                className="primary-button primary-button--modal"
                                disabled={isConnecting}
                                onClick={() => void confirmPassword()}
                            >
                                {isConnecting ? "Запуск..." : "Подтвердить"}
                            </button>
                        </div>
                    </section>
                </div>
            ) : null}
        </main>
    );
}