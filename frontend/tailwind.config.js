/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['ui-sans-serif', 'system-ui', '-apple-system', 'BlinkMacSystemFont', 'SF Pro Text', 'Segoe UI', 'Microsoft YaHei', 'sans-serif']
      },
      colors: {
        canvas: 'var(--canvas)',
        surface: {
          DEFAULT: 'var(--surface)',
          elevated: 'var(--surface-elevated)',
          muted: 'var(--surface-muted)'
        },
        brand: {
          DEFAULT: 'var(--accent)',
          strong: 'var(--accent-strong)'
        },
        finance: {
          navy: '#1E3A5F',
          blue: '#2E5FA3',
          bg: '#111827',
          panel: '#172033',
          muted: '#8EA3B8'
        }
      },
      boxShadow: {
        glow: '0 0 32px rgba(34, 184, 221, 0.16)',
        panel: '0 18px 50px rgba(0, 0, 0, 0.22)',
        float: '0 24px 80px rgba(0, 0, 0, 0.42)'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0', transform: 'translateY(4px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        }
      },
      animation: {
        'fade-in': 'fadeIn 180ms cubic-bezier(0.23, 1, 0.32, 1) forwards'
      }
    }
  },
  plugins: []
};
