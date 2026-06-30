/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        finance: {
          navy: '#1E3A5F',
          blue: '#2E5FA3',
          bg: '#111827',
          panel: '#172033',
          muted: '#8EA3B8'
        }
      },
      boxShadow: {
        glow: '0 0 40px rgba(46, 95, 163, 0.25)'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0', transform: 'translateY(6px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        }
      },
      animation: {
        'fade-in': 'fadeIn 100ms ease forwards'
      }
    }
  },
  plugins: []
};
