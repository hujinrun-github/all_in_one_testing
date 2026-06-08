import { render, screen } from '@testing-library/react';
import App from './App';

describe('App', () => {
  it('shows the project workspace navigation', () => {
    render(<App />);

    expect(screen.getByRole('heading', { name: 'All-in-One Testing' })).toBeInTheDocument();
    expect(screen.getByRole('navigation', { name: 'Project workspace' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Dashboard' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Targets & Agents' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Scenarios' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Runs' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Reports' })).toBeInTheDocument();
  });
});
