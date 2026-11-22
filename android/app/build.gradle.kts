plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
    id("org.jetbrains.kotlin.plugin.serialization")
    id("app.cash.paparazzi")
}

android {
    namespace = "com.telecyborg"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.telecyborg"
        minSdk = 29
        targetSdk = 35
        versionCode = 1
        versionName = "1.1.0"
    }

    signingConfigs {
        // Signed with the debug key unless TELECYBORG_KEYSTORE / _PASSWORD / _ALIAS are set.
        create("release") {
            val ks = System.getenv("TELECYBORG_KEYSTORE")
            if (ks != null) {
                storeFile = file(ks)
                storePassword = System.getenv("TELECYBORG_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("TELECYBORG_KEY_ALIAS")
                keyPassword = System.getenv("TELECYBORG_KEYSTORE_PASSWORD")
            } else {
                initWith(getByName("debug"))
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            signingConfig = signingConfigs.getByName("release")
        }
    }

    buildFeatures { compose = true }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
}

dependencies {
    implementation("androidx.core:core-ktx:1.15.0")
    implementation("androidx.activity:activity-compose:1.9.3")
    implementation(platform("androidx.compose:compose-bom:2024.12.01"))
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.foundation:foundation")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3")
    testImplementation("junit:junit:4.13.2")
}

android.testOptions.unitTests.isReturnDefaultValues = true
