plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
}

// Fassung kommt beim Bau aus BONDR_FASSUNG (Tag v3.3.0 → 3.3.0)
val fassung = (System.getenv("BONDR_FASSUNG") ?: "0.0.0").ifBlank { "0.0.0" }
val teile = fassung.split(".").map { it.toIntOrNull() ?: 0 } + listOf(0, 0, 0)

android {
    namespace = "io.github.dschonas04.bondr"
    compileSdk = 36

    defaultConfig {
        applicationId = "io.github.dschonas04.bondr"
        minSdk = 29
        targetSdk = 35
        versionCode = teile[0] * 10000 + teile[1] * 100 + teile[2]
        versionName = fassung
    }

    // Fester Schlüssel für Releases aus der Umgebung, sonst der Debug-Schlüssel
    val schluessel = System.getenv("BONDR_KEYSTORE")
    signingConfigs {
        if (schluessel != null && file(schluessel).exists()) {
            create("veroeffentlichen") {
                storeFile = file(schluessel)
                storePassword = System.getenv("BONDR_KEYSTORE_PASSWORT")
                keyAlias = System.getenv("BONDR_KEY_ALIAS") ?: "bondr"
                keyPassword = System.getenv("BONDR_KEYSTORE_PASSWORT")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = signingConfigs.findByName("veroeffentlichen") ?: signingConfigs.getByName("debug")
        }
    }

    // Der Go-Kern liegt als libbondrd.so in jniLibs und wird als Programm gestartet;
    // dafür muss er entpackt im Bibliotheksordner liegen.
    packaging {
        jniLibs {
            useLegacyPackaging = true
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        compose = true
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.activity.compose)
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.ui)
    implementation(libs.androidx.ui.graphics)
    implementation(libs.androidx.material3)
    implementation(libs.androidx.material.icons.extended)
    testImplementation(libs.junit)
}
