import 'package:flutter/material.dart';

class ArcTheme {
  const ArcTheme._();

  static const _bodyFont = 'Manrope';
  static const _headingFont = 'SpaceGrotesk';

  static ThemeData light() {
    final scheme = ColorScheme.fromSeed(
      seedColor: const Color(0xFF1E6FFF),
      secondary: const Color(0xFF005B9E),
      error: const Color(0xFFD0342C),
      brightness: Brightness.light,
      surface: const Color(0xFFF7F9FC),
    );

    final base = ThemeData(
      colorScheme: scheme,
      useMaterial3: true,
      scaffoldBackgroundColor: Colors.transparent,
    );

    return _withShared(base, scheme);
  }

  static ThemeData dark() {
    final scheme = ColorScheme.fromSeed(
      seedColor: const Color(0xFF66A1FF),
      secondary: const Color(0xFF8CBFFF),
      error: const Color(0xFFFF786C),
      brightness: Brightness.dark,
      surface: const Color(0xFF0B121C),
    );

    final base = ThemeData(
      colorScheme: scheme,
      useMaterial3: true,
      scaffoldBackgroundColor: Colors.transparent,
    );

    return _withShared(base, scheme);
  }

  static ThemeData _withShared(ThemeData base, ColorScheme scheme) {
    return base.copyWith(
      textTheme: _textTheme(base.textTheme, scheme),
      appBarTheme: AppBarTheme(
        backgroundColor: Colors.transparent,
        elevation: 0,
        centerTitle: false,
        foregroundColor: scheme.onSurface,
      ),
      cardTheme: CardThemeData(
        color: scheme.surface.withValues(alpha: 0.72),
        elevation: 0,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(24)),
      ),
      inputDecorationTheme: _inputTheme(scheme),
      elevatedButtonTheme: _elevatedButtonTheme(scheme),
      outlinedButtonTheme: _outlinedButtonTheme(scheme),
      textButtonTheme: _textButtonTheme(scheme),
      checkboxTheme: _checkboxTheme(scheme),
      segmentedButtonTheme: _segmentedButtonTheme(scheme),
      progressIndicatorTheme: ProgressIndicatorThemeData(color: scheme.primary),
      snackBarTheme: SnackBarThemeData(
        behavior: SnackBarBehavior.floating,
        backgroundColor: scheme.onSurface,
        contentTextStyle: TextStyle(
          fontFamily: _bodyFont,
          color: scheme.surface,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }

  static TextTheme _textTheme(TextTheme textTheme, ColorScheme scheme) {
    final body = textTheme.apply(
      fontFamily: _bodyFont,
      bodyColor: scheme.onSurface,
      displayColor: scheme.onSurface,
    );
    return body.copyWith(
      displaySmall: TextStyle(
        fontFamily: _headingFont,
        fontWeight: FontWeight.w700,
        letterSpacing: -0.8,
        color: scheme.onSurface,
      ),
      headlineMedium: TextStyle(
        fontFamily: _headingFont,
        fontWeight: FontWeight.w700,
        letterSpacing: -0.4,
        color: scheme.onSurface,
      ),
      titleLarge: TextStyle(
        fontFamily: _headingFont,
        fontWeight: FontWeight.w700,
        letterSpacing: -0.2,
        color: scheme.onSurface,
      ),
      titleMedium: body.titleMedium?.copyWith(
        color: scheme.onSurface,
        fontWeight: FontWeight.w700,
      ),
      bodyLarge: body.bodyLarge?.copyWith(
        height: 1.5,
        color: scheme.onSurface.withValues(alpha: 0.92),
      ),
      bodyMedium: body.bodyMedium?.copyWith(
        height: 1.45,
        color: scheme.onSurface.withValues(alpha: 0.84),
      ),
      bodySmall: body.bodySmall?.copyWith(
        height: 1.4,
        color: scheme.onSurface.withValues(alpha: 0.72),
      ),
      labelLarge: body.labelLarge?.copyWith(
        fontWeight: FontWeight.w700,
        color: scheme.onSurface,
      ),
      labelMedium: body.labelMedium?.copyWith(
        fontWeight: FontWeight.w600,
        color: scheme.onSurface.withValues(alpha: 0.84),
      ),
    );
  }

  static InputDecorationTheme _inputTheme(ColorScheme scheme) {
    final borderColor = scheme.onSurface.withValues(alpha: 0.14);
    return InputDecorationTheme(
      isDense: true,
      filled: true,
      fillColor: scheme.surface.withValues(alpha: 0.76),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(14),
        borderSide: BorderSide(color: borderColor),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(14),
        borderSide: BorderSide(color: borderColor),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(14),
        borderSide: BorderSide(color: scheme.primary, width: 1.4),
      ),
      errorBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(14),
        borderSide: BorderSide(color: scheme.error, width: 1.2),
      ),
      focusedErrorBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(14),
        borderSide: BorderSide(color: scheme.error, width: 1.4),
      ),
      contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 13),
      hintStyle: TextStyle(
        color: scheme.onSurface.withValues(alpha: 0.56),
        fontWeight: FontWeight.w500,
      ),
      labelStyle: TextStyle(
        color: scheme.onSurface.withValues(alpha: 0.74),
        fontWeight: FontWeight.w600,
      ),
    );
  }

  static ElevatedButtonThemeData _elevatedButtonTheme(ColorScheme scheme) {
    return ElevatedButtonThemeData(
      style: ElevatedButton.styleFrom(
        backgroundColor: scheme.primary,
        foregroundColor: scheme.onPrimary,
        minimumSize: const Size(0, 50),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
        textStyle: TextStyle(
          fontFamily: _bodyFont,
          fontWeight: FontWeight.w700,
          fontSize: 14,
        ),
      ),
    );
  }

  static OutlinedButtonThemeData _outlinedButtonTheme(ColorScheme scheme) {
    return OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: scheme.onSurface,
        minimumSize: const Size(0, 50),
        side: BorderSide(color: scheme.onSurface.withValues(alpha: 0.16)),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
        textStyle: TextStyle(
          fontFamily: _bodyFont,
          fontWeight: FontWeight.w700,
          fontSize: 14,
        ),
      ),
    );
  }

  static TextButtonThemeData _textButtonTheme(ColorScheme scheme) {
    return TextButtonThemeData(
      style: TextButton.styleFrom(
        foregroundColor: scheme.primary,
        textStyle: const TextStyle(
          fontFamily: _bodyFont,
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }

  static CheckboxThemeData _checkboxTheme(ColorScheme scheme) {
    return CheckboxThemeData(
      visualDensity: VisualDensity.compact,
      fillColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.selected)) {
          return scheme.primary;
        }
        return scheme.surface;
      }),
      side: BorderSide(color: scheme.onSurface.withValues(alpha: 0.24)),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(5)),
    );
  }

  static SegmentedButtonThemeData _segmentedButtonTheme(ColorScheme scheme) {
    return SegmentedButtonThemeData(
      style: ButtonStyle(
        foregroundColor: WidgetStateProperty.resolveWith((states) {
          if (states.contains(WidgetState.selected)) {
            return scheme.onPrimary;
          }
          return scheme.onSurface;
        }),
        backgroundColor: WidgetStateProperty.resolveWith((states) {
          if (states.contains(WidgetState.selected)) {
            return scheme.primary;
          }
          return scheme.surface.withValues(alpha: 0.62);
        }),
        side: WidgetStateProperty.all(
          BorderSide(color: scheme.onSurface.withValues(alpha: 0.15)),
        ),
        shape: WidgetStateProperty.all(
          RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        ),
      ),
    );
  }
}
