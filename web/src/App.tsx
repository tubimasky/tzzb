import { BrowserRouter, NavLink, Route, Routes } from 'react-router-dom';
import BrandMark from './components/BrandMark';
import AccountsPage from './pages/AccountsPage';
import AnalysisPage from './pages/AnalysisPage';
import CalendarPage from './pages/CalendarPage';
import DetailPage from './pages/DetailPage';
import Positions from './pages/Positions';
import StockProfitPage from './pages/StockProfitPage';
import './App.css';

const NAV_ITEMS = [
  { to: '/', label: '账户总览', end: true },
  { to: '/positions', label: '持仓明细' },
  { to: '/calendar', label: '盈亏日历' },
  { to: '/stockprofit', label: '股票盈亏' },
  { to: '/analysis', label: '仓位分析' },
];

function App() {
  return (
    <BrowserRouter>
      <div className="app">
        <nav className="nav" aria-label="主导航">
          <div className="nav-brand">
            <BrandMark className="nav-mark" />
            <span className="nav-logo">投资账本</span>
            <span className="nav-badge">Local</span>
          </div>
          <div className="nav-links">
            {NAV_ITEMS.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                className={({ isActive }) => (isActive ? 'nav-link active' : 'nav-link')}
              >
                {item.label}
              </NavLink>
            ))}
          </div>
        </nav>

        <main className="main">
          <Routes>
            <Route path="/" element={<AccountsPage />} />
            <Route path="/positions" element={<Positions />} />
            <Route path="/calendar" element={<CalendarPage />} />
            <Route path="/stockprofit" element={<StockProfitPage />} />
            <Route path="/analysis" element={<AnalysisPage />} />
            <Route path="/detail/:code" element={<DetailPage />} />
          </Routes>
        </main>

        <footer className="footer">
          <p>投资账本 · 本地私有数据，仅你可见</p>
        </footer>
      </div>
    </BrowserRouter>
  );
}

export default App;
