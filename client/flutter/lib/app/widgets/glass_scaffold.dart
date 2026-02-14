import 'dart:math' as math;
import 'dart:ui';

import 'package:flutter/material.dart';

class ArcGlassScaffold extends StatelessWidget {
  const ArcGlassScaffold({
    required this.child,
    super.key,
    this.hero,
    this.heroCollapsed,
  });

  final Widget child;
  final Widget? hero;
  final Widget? heroCollapsed;

  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;

    return Scaffold(
      body: Stack(
        children: [
          Positioned.fill(
            child: DecoratedBox(
              decoration: BoxDecoration(
                gradient: LinearGradient(
                  begin: Alignment.topLeft,
                  end: Alignment.bottomRight,
                  colors: [
                    colorScheme.primary.withValues(alpha: 0.24),
                    colorScheme.tertiary.withValues(alpha: 0.15),
                    colorScheme.surface.withValues(alpha: 0.92),
                  ],
                ),
              ),
            ),
          ),
          const _BackgroundBlobs(),
          SafeArea(
            child: LayoutBuilder(
              builder: (context, constraints) {
                final width = constraints.maxWidth;
                final horizontal = width >= 1200
                    ? 48.0
                    : width >= 900
                    ? 34.0
                    : width >= 640
                    ? 20.0
                    : 12.0;
                final vertical = width >= 900 ? 22.0 : 10.0;
                final showHero = width >= 980 && hero != null;
                final compactPanel = width < 640;
                final panelWidth = width >= 1320
                    ? 560.0
                    : width >= 980
                    ? 500.0
                    : width >= 640
                    ? 580.0
                    : width;

                if (showHero) {
                  return Padding(
                    padding: EdgeInsets.symmetric(
                      horizontal: horizontal,
                      vertical: vertical,
                    ),
                    child: Row(
                      children: [
                        Expanded(
                          child: Padding(
                            padding: const EdgeInsets.only(right: 22),
                            child: hero,
                          ),
                        ),
                        Align(
                          alignment: Alignment.center,
                          child: ConstrainedBox(
                            constraints: BoxConstraints(maxWidth: panelWidth),
                            child: _GlassPanel(
                              compact: compactPanel,
                              child: child,
                            ),
                          ),
                        ),
                      ],
                    ),
                  );
                }

                return Padding(
                  padding: EdgeInsets.symmetric(
                    horizontal: horizontal,
                    vertical: vertical,
                  ),
                  child: Column(
                    children: [
                      if (heroCollapsed != null) ...[
                        heroCollapsed!,
                        const SizedBox(height: 12),
                      ],
                      Expanded(
                        child: Align(
                          alignment: Alignment.topCenter,
                          child: ConstrainedBox(
                            constraints: BoxConstraints(maxWidth: panelWidth),
                            child: _GlassPanel(
                              compact: compactPanel,
                              child: child,
                            ),
                          ),
                        ),
                      ),
                    ],
                  ),
                );
              },
            ),
          ),
        ],
      ),
    );
  }
}

class ArcHeroPanel extends StatelessWidget {
  const ArcHeroPanel({super.key});

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final colorScheme = Theme.of(context).colorScheme;

    return Align(
      alignment: Alignment.centerLeft,
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 520),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 22),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const _ArcMark(size: 88),
              const SizedBox(height: 24),
              Text('Arc Messenger', style: textTheme.displaySmall),
              const SizedBox(height: 12),
              Text(
                'Calm, secure, responsive messaging.\nInvite-based auth with modern session security.',
                style: textTheme.bodyLarge?.copyWith(
                  color: colorScheme.onSurface.withValues(alpha: 0.86),
                ),
              ),
              const SizedBox(height: 28),
              Wrap(
                spacing: 10,
                runSpacing: 10,
                children: const [
                  _HeroChip(label: 'Web + Native Auth'),
                  _HeroChip(label: 'Cookie + CSRF Ready'),
                  _HeroChip(label: 'Token Rotation'),
                  _HeroChip(label: 'Realtime-Prepared'),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class ArcHeroCompact extends StatelessWidget {
  const ArcHeroCompact({super.key});

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;

    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: scheme.outline.withValues(alpha: 0.28)),
        color: scheme.surface.withValues(alpha: 0.45),
      ),
      child: Row(
        children: [
          const _ArcMark(size: 42),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Arc Auth Client', style: textTheme.titleMedium),
                Text(
                  'Secure invite/signup/login flow',
                  style: textTheme.bodySmall,
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _GlassPanel extends StatelessWidget {
  const _GlassPanel({required this.child, required this.compact});

  final Widget child;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    final radius = compact ? 22.0 : 30.0;
    final blur = compact ? 14.0 : 20.0;
    final padding = compact ? 16.0 : 24.0;

    return ClipRRect(
      borderRadius: BorderRadius.circular(radius),
      child: BackdropFilter(
        filter: ImageFilter.blur(sigmaX: blur, sigmaY: blur),
        child: DecoratedBox(
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(radius),
            border: Border.all(
              color: colorScheme.onSurface.withValues(alpha: 0.14),
            ),
            gradient: LinearGradient(
              begin: Alignment.topLeft,
              end: Alignment.bottomRight,
              colors: [
                colorScheme.surface.withValues(alpha: 0.86),
                colorScheme.surfaceContainerHighest.withValues(alpha: 0.58),
              ],
            ),
            boxShadow: [
              BoxShadow(
                color: colorScheme.shadow.withValues(alpha: 0.16),
                blurRadius: 26,
                offset: const Offset(0, 12),
              ),
            ],
          ),
          child: Padding(padding: EdgeInsets.all(padding), child: child),
        ),
      ),
    );
  }
}

class _BackgroundBlobs extends StatefulWidget {
  const _BackgroundBlobs();

  @override
  State<_BackgroundBlobs> createState() => _BackgroundBlobsState();
}

class _BackgroundBlobsState extends State<_BackgroundBlobs>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(
      vsync: this,
      duration: const Duration(seconds: 20),
    )..repeat(reverse: true);
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;

    return IgnorePointer(
      child: AnimatedBuilder(
        animation: _controller,
        builder: (context, child) {
          final t = Curves.easeInOut.transform(_controller.value);
          final shiftA = (t - 0.5) * 36;
          final shiftB = (0.5 - t) * 42;
          final pulse = (math.sin(t * math.pi * 2) + 1) / 2;

          return Stack(
            children: [
              Positioned(
                top: -120 + shiftA,
                left: -100 + shiftB,
                child: _blob(
                  360,
                  scheme.primary.withValues(alpha: 0.09 + pulse * 0.05),
                ),
              ),
              Positioned(
                right: -80 + shiftA,
                top: 88 + shiftB,
                child: _blob(
                  300,
                  scheme.tertiary.withValues(alpha: 0.08 + pulse * 0.04),
                ),
              ),
              Positioned(
                bottom: -120 + shiftB,
                left: 140 + shiftA,
                child: _blob(
                  360,
                  scheme.secondary.withValues(alpha: 0.08 + pulse * 0.05),
                ),
              ),
            ],
          );
        },
      ),
    );
  }

  Widget _blob(double size, Color color) {
    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(color: color, shape: BoxShape.circle),
    );
  }
}

class _ArcMark extends StatelessWidget {
  const _ArcMark({required this.size});

  final double size;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;

    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(size * 0.28),
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [scheme.primary, scheme.tertiary],
        ),
        boxShadow: [
          BoxShadow(
            color: scheme.primary.withValues(alpha: 0.4),
            blurRadius: 24,
            offset: const Offset(0, 12),
          ),
        ],
      ),
      child: Icon(
        Icons.forum_rounded,
        size: size * 0.48,
        color: scheme.onPrimary,
      ),
    );
  }
}

class _HeroChip extends StatelessWidget {
  const _HeroChip({required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: scheme.surface.withValues(alpha: 0.45),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: scheme.outline.withValues(alpha: 0.26)),
      ),
      child: Text(label, style: Theme.of(context).textTheme.labelLarge),
    );
  }
}
